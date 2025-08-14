package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/CanobbioE/poly-route/example/mock"
)

func main() {
	servers := []any{
		mock.StartMockRegionRepository("localhost:1234"),
		mock.StartMockHTTPBackend("localhost:8085", "eu"),
		mock.StartMockHTTPBackend("localhost:8081", "us"),
		mock.StartMockGRPCBackend("localhost:9095", "eu"),
		mock.StartMockGRPCBackend("localhost:9091", "us"),
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down servers...")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	for _, srv := range servers {
		if srv != nil {
			if v, isHTTP := srv.(*http.Server); isHTTP {
				_ = v.Shutdown(ctx)
			}
			if v, isGrpc := srv.(*grpc.Server); isGrpc {
				v.GracefulStop()
			}
		}
	}

	log.Println("All servers stopped.")
}
