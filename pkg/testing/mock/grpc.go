package mock

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"log"
	"net"
)

func startMockGRPCBackend(addr string) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("mock grpc listen %s failed: %v", addr, err)
	}
	server := grpc.NewServer()
	reflection.Register(server)
	// Register a generic service that simply echoes messages using RegisterService
	// We register service "mypkg.MyService" with a single method "Authorize" and "Read"
	serviceDesc := &grpc.ServiceDesc{
		ServiceName: "mypkg.MyService",
		HandlerType: (*struct{})(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "Authorize",
				Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
					var b []byte
					if err := dec(&b); err != nil {
						return nil, err
					}
					// echo back
					return b, nil
				},
			},
			{
				MethodName: "Read",
				Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
					var b []byte
					if err := dec(&b); err != nil {
						return nil, err
					}
					return append([]byte("echo:"), b...), nil
				},
			},
		},
		Streams: []grpc.StreamDesc{},
	}
	server.RegisterService(serviceDesc, struct{}{})
	log.Printf("mock grpc backend listening on %s", addr)
	go func() {
		if err := server.Serve(lis); err != nil {
			log.Fatalf("mock grpc backend failed: %v", err)
		}
	}()
}
