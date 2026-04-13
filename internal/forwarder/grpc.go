package forwarder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/CanobbioE/poly-route/internal/codec"
	"github.com/CanobbioE/poly-route/internal/config"
	"github.com/CanobbioE/poly-route/internal/logger"
	"github.com/CanobbioE/poly-route/internal/routing"
)

// MetadataRegionKey is the metadata key used to retrieve the region from the api call.
const MetadataRegionKey = "poly-route-region"

// framePool is used by forwardStream to reuse slices of bytes to ease GC stress.
var framePool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 32*1024) // 32 KB
		return &b
	},
}

// GRPCForwarder implements a reverse transparent proxy for gRPC.
type GRPCForwarder struct {
	cfg            *config.ProtocolCfg
	regionResolver routing.RegionResolver
	log            logger.LazyLogger
	pool           *ConnectionPool
	routes         []*routing.CompiledRoute
}

// GRPC creates a new GRPCForwarder with an internal connection pool.
// Connections are dialed lazily and reused across requests.
func GRPC(cfg *config.ProtocolCfg, resolver routing.RegionResolver, l logger.LazyLogger) *GRPCForwarder {
	pool := NewConnectionPool(
		// Using insecure credentials: mTLS is assumed to be handled at the service-mesh/infrastructure layer.
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(&codec.PassThrough{})),
	)

	return &GRPCForwarder{
		cfg:            cfg,
		regionResolver: resolver,
		log:            l,
		pool:           pool,
		routes:         routing.CompileRoutes(cfg, config.ProtocolGRPC),
	}
}

// Close drains the connection pool. It should be called once during graceful shutdown,
// after the gRPC server has stopped accepting new requests.
func (x *GRPCForwarder) Close() error {
	return x.pool.CloseAll()
}

// Handler returns a [grpc.StreamHandler] transparent reverse proxy.
func (x *GRPCForwarder) Handler() grpc.StreamHandler {
	return func(_ any, stream grpc.ServerStream) error {
		method, ok := grpc.MethodFromServerStream(stream)
		if !ok {
			return errors.New("cannot get method from stream")
		}

		// copy context
		incomingCtx := stream.Context()
		md, _ := metadata.FromIncomingContext(incomingCtx)
		outgoingCtx := metadata.NewOutgoingContext(incomingCtx, md.Copy())

		// resolve region from metadata
		var region string
		if vals := md.Get(MetadataRegionKey); len(vals) > 0 {
			region = vals[0]
		}
		if region == "" {
			return fmt.Errorf("no region found in metadata, set %s", MetadataRegionKey)
		}

		resolvedRegion, err := x.regionResolver.ResolveRegion(outgoingCtx, region)
		if err != nil {
			return fmt.Errorf("cannot resolve region %s: %w", region, err)
		}

		backend, ok := x.FindBackend(method, resolvedRegion)
		if !ok {
			return fmt.Errorf("no backend for method %s region %s", method, region)
		}

		if err = x.forwardGRPCStream(outgoingCtx, backend, method, stream); err != nil {
			return fmt.Errorf("forward grpc stream: %w", err)
		}
		return nil
	}
}

func (x *GRPCForwarder) forwardGRPCStream(
	ctx context.Context,
	backendAddr, method string,
	serverStream grpc.ServerStream,
) error {
	lazyLog := x.log.WithLazy("method", method, "address", backendAddr)
	lazyLog.Info("forwarding grpc stream")

	// Fetch a cached connection from the pool instead of dialling on every request.
	// The pool dials lazily and only replaces a connection when it has been shut down.
	conn, err := x.pool.Get(backendAddr)
	if err != nil {
		return fmt.Errorf("get backend connection %s: %w", backendAddr, err)
	}
	// Note: we do NOT close conn here. Ownership stays with the pool.

	desc := &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}

	clientCtx, clientCancel := context.WithCancel(ctx)
	defer clientCancel()

	clientStream, err := conn.NewStream(clientCtx, desc, method)
	if err != nil {
		return fmt.Errorf("create backend stream: %w", err)
	}

	// Bidirectional copy.
	cli2SrvErrCh := forwardStream(ctx, clientStream, serverStream)
	srv2CliErrCh := forwardStream(ctx, serverStream, clientStream)

	for range 2 {
		select {
		case s2cErr := <-srv2CliErrCh:
			if !errors.Is(s2cErr, io.EOF) {
				clientCancel()
				return status.Errorf(codes.Internal, "failed proxying server -> client: %v", s2cErr)
			}
			if err = clientStream.CloseSend(); err != nil {
				return status.Errorf(codes.Internal, "failed proxying client: %v", err)
			}
		case c2sErr := <-cli2SrvErrCh:
			serverStream.SetTrailer(clientStream.Trailer())
			if !errors.Is(c2sErr, io.EOF) {
				return c2sErr
			}
			return nil
		}
	}

	// Should never be reached; log as a bug for observability.
	lazyLog.Error("gRPC proxying reached an invalid state", "bug", true)
	return nil
}

type senderReceiver interface {
	SendMsg(m any) error
	RecvMsg(m any) error
}

func forwardStream[S senderReceiver, D senderReceiver](ctx context.Context, src S, dst D) chan error {
	errCh := make(chan error, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			default:
				// Get a buffer from the pool
				// and put it back before exiting
				bufPtr := framePool.Get().(*[]byte)
				raw := (*bufPtr)[:0]

				if err := src.RecvMsg(&raw); err != nil {
					framePool.Put(bufPtr)
					errCh <- err
					return
				}
				if err := dst.SendMsg(&raw); err != nil {
					framePool.Put(bufPtr)
					errCh <- err
					return
				}
				framePool.Put(bufPtr)
			}
		}
	}()
	return errCh
}

// FindBackend finds a GRPC backend by best match using the GRPCForwarder protocol configuration.
// The best route is eiter an exact match with the entrypoint or a wildcard-suffixed match.
func (x *GRPCForwarder) FindBackend(entrypoint, region string) (string, bool) {
	for _, r := range x.routes {
		switch r.Kind {
		case routing.RouteExact:
			if r.Prefix != entrypoint {
				continue
			}
		case routing.RoutePrefix:
			// r.prefix is e.g. "/pkg.Service/" — entrypoint must start with it
			// and the last segment must sit right after it (no further slashes).
			if !strings.HasPrefix(entrypoint, r.Prefix) {
				continue
			}
			rest := entrypoint[len(r.Prefix):]
			if strings.Contains(rest, "/") {
				continue // only single-segment wildcard in gRPC
			}
		case routing.RouteMatchAll:
			// always matches
		}

		dest, ok := r.Mappings[region]
		if !ok {
			return "", false
		}

		if r.Kind == routing.RouteExact {
			return dest, true
		}
		// Both RoutePrefix and RouteMatchAll append the suffix.
		var suffix string
		if r.Kind == routing.RoutePrefix {
			suffix = entrypoint[len(r.Prefix)-1:] // include the leading "/"
		} else {
			suffix = entrypoint
		}
		return path.Join(dest, suffix), true
	}
	return "", false
}
