package main

import (
	"github.com/CanobbioE/poly-route/pkg/config"
	"github.com/CanobbioE/poly-route/pkg/frontends"
	"log"
	"net"
	"net/http"

	"google.golang.org/grpc"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		http.HandleFunc("/", frontends.HTTP(cfg))
		log.Println("REST listening on", ":"+cfg.HTTP.Listen)
		log.Fatal(http.ListenAndServe(":"+cfg.HTTP.Listen, nil))
	}()

	lis, err := net.Listen("tcp", ":"+cfg.GRPC.Listen)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer(grpc.UnknownServiceHandler(frontends.GRPC(cfg)))
	log.Println("gRPC proxy listening on :" + cfg.GRPC.Listen)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
