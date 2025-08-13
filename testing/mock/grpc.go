package mock

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"strconv"

	"google.golang.org/grpc"

	pb "github.com/CanobbioE/poly-route/testing/mock/proto-gen/mockserver/v1"
)

// mockServer implements [pb.MockServiceServer].
type mockServer struct {
	pb.UnimplementedMockServiceServer
	name string
}

// Invoke mocks a gRPC call where the client is sending a single request and the server is returning a single response.
func (*mockServer) Invoke(_ context.Context, req *pb.ReadRequest) (*pb.ReadResponse, error) {
	return &pb.ReadResponse{Data: "Response for " + req.ResourceId}, nil
}

// BiDirectionalStream mocks a gRPC call where the client is sending a stream and the server is returning a stream.
func (*mockServer) BiDirectionalStream(stream pb.MockService_BiDirectionalStreamServer) error {
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		resp := &pb.ReadResponse{Data: "Stream response for " + req.ResourceId}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

// ClientStream mocks a gRPC call where the client is sending a stream.
func (*mockServer) ClientStream(stream pb.MockService_ClientStreamServer) error {
	var combinedData string
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return stream.SendAndClose(&pb.ReadResponse{Data: combinedData})
		}
		if err != nil {
			return err
		}
		combinedData += req.ResourceId + ";"
	}
}

// ServerStream mocks a gRPC call where the server is returning a stream.
func (*mockServer) ServerStream(req *pb.ReadRequest, stream pb.MockService_ServerStreamServer) error {
	for i := range 5 {
		resp := &pb.ReadResponse{Data: req.ResourceId + " response #" + strconv.Itoa(i)}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
	return nil
}

// StartMockGRPCBackend is a fire-and-forget function that starts a mock gRPC server.
// The server implements all methods of [pb.MockServiceServer].
func StartMockGRPCBackend(addr, name string) *grpc.Server {
	lis, err := net.Listen("tcp", addr) //nolint:noctx // test files, there's no context
	if err != nil {
		log.Fatalf("failed to listen %s (%s): %v", addr, name, err) //nolint:revive // test files can deep-exit
	}

	grpcServer := grpc.NewServer()
	pb.RegisterMockServiceServer(grpcServer, &mockServer{name: name})

	log.Printf("mock gRPC server listening on %s", addr)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("mock http backend failed to serve: %s", err) //nolint:revive // test files can deep-exit
		}
	}()

	return grpcServer
}
