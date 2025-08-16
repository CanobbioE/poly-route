package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/CanobbioE/poly-route/internal/codec"
	"github.com/CanobbioE/poly-route/internal/config"
	"github.com/CanobbioE/poly-route/internal/forwarder"
	"github.com/CanobbioE/poly-route/internal/routing"
)

func main() {
	cfgPath := os.Getenv("CONFIG_FILE_PATH")
	if cfgPath == "" {
		log.Println("couldn't read configuration filepath as env variable, please set CONFIG_FILE_PATH")
		log.Println("defaulting to config.yaml")
		cfgPath = "config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("could not read config file at %s: %v", cfgPath, err)
	}

	regionResolver, err := routing.NewResolver(cfg.RegionRetriever)
	if err != nil {
		log.Fatalf("failed to create region resolver %v", err)
	}

	httpProxy := startHTTPProxy(cfg, regionResolver)
	grpcProxy := startGRPCProxy(cfg, regionResolver)

	if httpProxy == nil && grpcProxy == nil {
		log.Fatal("no proxy server set up, stopping now")
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down proxies...")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if httpProxy != nil {
		err = httpProxy.Shutdown(ctx)
		if err != nil {
			log.Println("failed to shutdown proxy HTTP server")
		}
	}
	if grpcProxy != nil {
		grpcProxy.GracefulStop()
	}
}

func startHTTPProxy(cfg *config.ServiceCfg, resolver routing.RegionResolver) *http.Server {
	if cfg.HTTP == nil {
		log.Println("HTTP proxy disabled")
		return nil
	}
	var server *http.Server
	go func() {
		mux := http.NewServeMux()
		httpHandler := forwarder.HTTP(cfg.HTTP, resolver).Handler()
		mux.HandleFunc("/", httpHandler)

		server = &http.Server{
			Addr:              ":" + cfg.HTTP.Listen,
			Handler:           mux,
			ReadTimeout:       5 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       120 * time.Second,
			ReadHeaderTimeout: 2 * time.Second,
		}
		log.Println("http proxy listening on", server.Addr)
		if err := server.ListenAndServe(); err != nil {
			log.Printf("failed to serve http: %v", err)
			return
		}
	}()

	return server
}

func startGRPCProxy(cfg *config.ServiceCfg, resolver routing.RegionResolver) *grpc.Server {
	if cfg.GRPC == nil {
		log.Println("gRPC proxy disabled")
		return nil
	}

	var server *grpc.Server
	go func() {
		lis, err := net.Listen("tcp", ":"+cfg.GRPC.Listen) //nolint:noctx // test files, there's no context
		if err != nil {
			log.Printf("failed to listen to %s: %v", cfg.GRPC.Listen, err)
			return
		}

		grpcHandler := forwarder.GRPC(cfg.GRPC, resolver).Handler()
		server = grpc.NewServer(grpc.UnknownServiceHandler(grpcHandler), grpc.ForceServerCodec(&codec.PassThrough{}))
		log.Println("gRPC proxy listening on" + lis.Addr().String())
		if err := server.Serve(lis); err != nil {
			log.Printf("failed to serve grpc: %v", err)
			return
		}
	}()

	return server
}
