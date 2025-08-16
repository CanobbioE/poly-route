package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	pb "github.com/CanobbioE/poly-route/example/mock/proto-gen/mockserver/v1"
	"github.com/CanobbioE/poly-route/internal/forwarder"
)

var (
	polyRouteGRPCServerAddr string
	polyRouteHTPServerAddr  string
)

func init() {
	polyRouteHTPServerAddr = os.Getenv("HTTP_HOST")
	polyRouteGRPCServerAddr = os.Getenv("GRPC_HOST")
}

func main() {
	if err := testGrpcStream("europe-west1"); err != nil {
		log.Fatalf("failed to test against grpc stream europe-west1: %v", err)
	}
	if err := testGrpcStream("us-east1"); err != nil {
		log.Fatalf("failed to test against grpc stream us-east1: %v", err)
	}
	if err := testGrpcInvoke("europe-west1"); err != nil {
		log.Fatalf("failed to test against grpc invoke europe-west1: %v", err)
	}
	if err := testGrpcInvoke("us-east1"); err != nil {
		log.Fatalf("failed to test against grpc invoke us-east1: %v", err)
	}

	if err := testHTTPClient("europe-west1"); err != nil {
		log.Fatalf("failed to test against http europe-west1: %v", err)
	}
	if err := testHTTPClient("us-east1"); err != nil {
		log.Fatalf("failed to test against http us-east1: %v", err)
	}
}

func testGrpcStream(regionMappingKey string) error {
	conn, err := gRPCClient(polyRouteGRPCServerAddr)
	if err != nil {
		return err
	}
	client := pb.NewMockServiceClient(conn)
	defer conn.Close() //nolint:errcheck // test file omit err check for simplicity

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	ctx = metadata.NewOutgoingContext(ctx, metadata.MD{
		forwarder.MetadataRegionKey: []string{regionMappingKey},
	})

	biStream, err := client.BiDirectionalStream(ctx)
	if err != nil {
		return err
	}

	resourceIDs := []string{"res-123", "res-456", "res-789"}
	for _, id := range resourceIDs {
		if err = biStream.Send(&pb.ReadRequest{ResourceId: id}); err != nil {
			return fmt.Errorf("failed to send request: %w", err)
		}
		time.Sleep(100 * time.Millisecond) // just to space them out
	}

	if err = biStream.CloseSend(); err != nil {
		return fmt.Errorf("failed to close send: %w", err)
	}

	for {
		var resp *pb.ReadResponse
		resp, err = biStream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("error receiving response: %w", err)
		}
		log.Printf("Received BiDirectionalStream: %s", resp.GetData())
	}

	cliStream, err := client.ClientStream(ctx)
	if err != nil {
		return err
	}
	for _, id := range resourceIDs {
		if err = cliStream.Send(&pb.ReadRequest{ResourceId: id}); err != nil {
			return fmt.Errorf("failed to send request: %w", err)
		}
		time.Sleep(100 * time.Millisecond) // just to space them out
	}
	resp, err := cliStream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("failed to close send: %w", err)
	}

	log.Printf("Received ClientStream: %s", resp.GetData())

	srvStream, err := client.ServerStream(ctx, &pb.ReadRequest{ResourceId: "res-123"})
	if err != nil {
		return err
	}

	for {
		resp, err := srvStream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("error receiving response: %w", err)
		}
		log.Printf("Received ServerStream: %s", resp.GetData())
	}

	return nil
}

func testGrpcInvoke(regionMappingKey string) error {
	conn, err := gRPCClient(polyRouteGRPCServerAddr)
	if err != nil {
		return err
	}
	client := pb.NewMockServiceClient(conn)
	defer conn.Close() //nolint:errcheck // test file omit err check for simplicity

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	ctx = metadata.NewOutgoingContext(ctx, metadata.MD{
		forwarder.MetadataRegionKey: []string{regionMappingKey},
	})

	authResp, err := client.Invoke(ctx, &pb.ReadRequest{
		ResourceId: "res-123",
	})
	if err != nil {
		return err
	}

	log.Println(authResp)
	return nil
}

func gRPCClient(addr string) (*grpc.ClientConn, error) {
	// Set up a connection to the server.
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func testHTTPClient(regionHeader string) error {
	getReq, err := http.NewRequest(http.MethodGet, polyRouteHTPServerAddr, http.NoBody) //nolint:noctx // test file
	if err != nil {
		return err
	}

	postReq, err := http.NewRequest(http.MethodPost, polyRouteHTPServerAddr, http.NoBody) //nolint:noctx // test file
	if err != nil {
		return err
	}

	for _, req := range []*http.Request{getReq, postReq} {
		req.Header.Set(forwarder.HeaderRegionKey, regionHeader)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}

		out, err := io.ReadAll(resp.Body)
		if err != nil {
			_ = resp.Body.Close()
			return err
		}
		_ = resp.Body.Close()
		log.Println(string(out))
	}

	return nil
}
