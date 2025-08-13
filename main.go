package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"google.golang.org/grpc"

	"github.com/CanobbioE/poly-route/internal/codec"
	"github.com/CanobbioE/poly-route/internal/config"
	"github.com/CanobbioE/poly-route/internal/forwarder"
)

func main() {
	cfgPath := os.Getenv("CONFIG_FILE_PATH")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", forwarder.HTTP(cfg))

		server := &http.Server{
			Addr:              ":" + cfg.HTTP.Listen,
			Handler:           mux,
			ReadTimeout:       5 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       120 * time.Second,
			ReadHeaderTimeout: 2 * time.Second,
		}
		log.Fatal(server.ListenAndServe())
	}()

	lis, err := net.Listen("tcp", ":"+cfg.GRPC.Listen) //nolint:noctx // test files, there's no context
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcHandler, err := forwarder.GRPC(cfg)
	if err != nil {
		log.Fatalf("failed to create grpc handler: %v", err)
	}
	s := grpc.NewServer(grpc.UnknownServiceHandler(grpcHandler.Handler), grpc.ForceServerCodec(&codec.PassThrough{}))
	log.Println("gRPC proxy listening on :" + cfg.GRPC.Listen)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
