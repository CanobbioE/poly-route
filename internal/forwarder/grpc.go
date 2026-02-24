package forwarder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
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

// framePool is used by forwardStream to reuse slice of bytes to ease GC stress.
var framePool = sync.Pool{
	New: func() any {
		// initialize a pointer to a slice to avoid extra allocations
		b := make([]byte, 0, 32*1024) // 32KB
		return &b
	},
}

// GRPCForwarder implements a reverse transparent proxy for GRPC.
type GRPCForwarder struct {
	cfg            *config.ProtocolCfg
	regionResolver routing.RegionResolver
	log            logger.LazyLogger
}

// GRPC creates a new GRPCForwarder.
func GRPC(cfg *config.ProtocolCfg, resolver routing.RegionResolver, l logger.LazyLogger) *GRPCForwarder {
	return &GRPCForwarder{
		cfg:            cfg,
		regionResolver: resolver,
		log:            l,
	}
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

		err = x.forwardGRPCStream(outgoingCtx, backend, method, stream)
		if err != nil {
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

	conn, err := grpc.NewClient(backendAddr,
		// using insecure: assuming mTLS is handled at the service mesh/infrastructure layer
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(&codec.PassThrough{})))
	if err != nil {
		return fmt.Errorf("dial backend %s: %w", backendAddr, err)
	}
	defer func() {
		err = conn.Close()
		if err != nil {
			lazyLog.Warn("failed to close backend connection")
		}
	}()

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

	// Bidirectional copy
	cli2SrvErrCh := forwardStream(ctx, clientStream, serverStream)
	srv2CliErrCh := forwardStream(ctx, serverStream, clientStream)

	// listen for errors: io.EOF from either chan is good
	// anything else is an issue
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
	// this is an invalid state that should never occur, return nil but print a bug log line for observability
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
				// get a buffer from the pool
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
	mapping, exactMatch := x.cfg.Destinations[entrypoint]
	if exactMatch {
		v, ok := mapping[region]
		return v, ok
	}

	// if not found by full entrypoint, try matching by wildcard
	last := strings.LastIndex(entrypoint, "/")
	if last == -1 {
		return "", false
	}

	wildcard := entrypoint[:last+1] + "*"
	if m, match := x.cfg.Destinations[wildcard]; match {
		destination, ok := m[region]
		return filepath.Join(destination, entrypoint[len(wildcard)-2:]), ok
	}

	if m, matchAll := x.cfg.Destinations["*"]; matchAll {
		destination, ok := m[region]
		return filepath.Join(destination, entrypoint), ok
	}

	return "", false
}
