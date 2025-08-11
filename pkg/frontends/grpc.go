package frontends

import (
	"fmt"
	"github.com/CanobbioE/poly-route/pkg/config"
	"github.com/CanobbioE/poly-route/pkg/routing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"time"
)

func findGRPCBackend(cfg *config.ServiceCfg, method string, region string) (string, bool) {
	for _, d := range cfg.GRPC.Destinations {
		if d.Entrypoint == method {
			switch region {
			case "eu":
				return d.EU, true
			case "us":
				return d.US, true
			default:
				return "", false
			}
		}
	}
	return "", false
}

func GRPC(cfg *config.ServiceCfg) func(srv interface{}, stream grpc.ServerStream) error {
	return func(srv interface{}, stream grpc.ServerStream) error {
		method, ok := grpc.MethodFromServerStream(stream)
		if !ok {
			return fmt.Errorf("cannot get method from stream")
		}

		// resolve region from metadata
		md, _ := metadata.FromIncomingContext(stream.Context())
		var user string
		if vals := md.Get("user-id"); len(vals) > 0 {
			user = vals[0]
		}

		region, err := routing.GetRegion(cfg, user)
		if err != nil {
			return err
		}

		backend, ok := findGRPCBackend(cfg, method, region)
		if !ok {
			return fmt.Errorf("no backend for method %s region %s", method, region)
		}

		// Dial backend
		conn, err := grpc.Dial(backend, grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(3*time.Second))
		if err != nil {
			return fmt.Errorf("dial backend %s failed: %v", backend, err)
		}
		defer conn.Close()

		// Create client stream to backend
		clientStream, err := conn.NewStream(stream.Context(), &grpc.StreamDesc{StreamName: method, ServerStreams: true, ClientStreams: true}, method)
		if err != nil {
			return fmt.Errorf("create backend stream failed: %v", err)
		}

		done := make(chan error, 2)
		// client->backend
		go func() {
			for {
				m := new([]byte)
				if err := stream.RecvMsg(m); err != nil {
					done <- err
					return
				}
				if err := clientStream.SendMsg(m); err != nil {
					done <- err
					return
				}
			}
		}()
		// backend->client
		go func() {
			for {
				m := new([]byte)
				if err := clientStream.RecvMsg(m); err != nil {
					done <- err
					return
				}
				if err := stream.SendMsg(m); err != nil {
					done <- err
					return
				}
			}
		}()

		// wait for first error (including EOF)
		err = <-done
		return err
	}
}
