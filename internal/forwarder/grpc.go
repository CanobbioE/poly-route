package forwarder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/CanobbioE/poly-route/internal/codec"
	"github.com/CanobbioE/poly-route/internal/config"
	"github.com/CanobbioE/poly-route/internal/routing"
)

// MetadataRegionKey is the metadata key used to retrieve the region from the api call.
const MetadataRegionKey = "poly-route-region"

// GRPCForwarder implements a reverse transparent proxy for GRPC.
type GRPCForwarder struct {
	cfg            *config.ProtocolCfg
	regionResolver routing.RegionResolver
}

// GRPC creates a new GRPCForwarder.
func GRPC(cfg *config.ProtocolCfg, resolver routing.RegionResolver) *GRPCForwarder {
	return &GRPCForwarder{
		cfg:            cfg,
		regionResolver: resolver,
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

		backend, ok := x.findBackend(method, resolvedRegion)
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

func (*GRPCForwarder) forwardGRPCStream(
	ctx context.Context,
	backendAddr, method string,
	serverStream grpc.ServerStream,
) error {
	log.Printf("forwarding GRPC request towards addr=%s method=%s", backendAddr, method)

	conn, err := grpc.NewClient(backendAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(&codec.PassThrough{})))
	if err != nil {
		return fmt.Errorf("dial backend %s: %w", backendAddr, err)
	}
	defer func() {
		err = conn.Close()
		if err != nil {
			log.Printf("failed to close backend %s: %v", backendAddr, err)
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
	cli2SrvErrCh := forwardStream(clientStream, serverStream)
	srv2CliErrCh := forwardStream(serverStream, clientStream)

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
	// TODO: error message
	return status.Errorf(codes.Internal, "gRPC proxying should never reach this stage.")
}

func (x *GRPCForwarder) findBackend(method, region string) (string, bool) {
	for _, d := range x.cfg.Destinations {
		if d.Entrypoint == method {
			v, ok := d.Mapping[region]
			return v, ok
		}
	}
	return "", false
}

type senderReceiver interface {
	SendMsg(m any) error
	RecvMsg(m any) error
}

func forwardStream[S senderReceiver, D senderReceiver](src S, dst D) chan error {
	errCh := make(chan error, 1)
	go func() {
		// client → backend
		for {
			var raw []byte
			if err := src.RecvMsg(&raw); err != nil {
				errCh <- err
				return
			}
			if err := dst.SendMsg(&raw); err != nil {
				errCh <- err
				return
			}
		}
	}()

	return errCh
}
