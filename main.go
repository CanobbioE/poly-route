package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	graphQLProxy := startGraphQLProxy(cfg, regionResolver)

	if httpProxy == nil && grpcProxy == nil && graphQLProxy == nil {
		log.Fatal("no proxy server set up, stopping now")
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("shutting down proxies...")

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
	if graphQLProxy != nil {
		err = graphQLProxy.Shutdown(ctx)
		if err != nil {
			log.Println("failed to shutdown proxy GraphQL server")
		}
	}
}

func startGraphQLProxy(cfg *config.ServiceCfg, resolver routing.RegionResolver) *http.Server {
	if cfg.GraphQL == nil {
		log.Println("GraphQL proxy not enabled")
		return nil
	}

	server := newHTTPProxyServer(cfg.GraphQL, resolver)

	go func() {
		log.Println("graphql proxy listening on", server.Addr)
		if err := server.ListenAndServe(); err != nil {
			log.Printf("failed to serve graphql over http: %v", err)
			return
		}
	}()

	return server
}

func startHTTPProxy(cfg *config.ServiceCfg, resolver routing.RegionResolver) *http.Server {
	if cfg.HTTP == nil {
		log.Println("HTTP proxy not enabled")
		return nil
	}

	server := newHTTPProxyServer(cfg.HTTP, resolver)

	go func() {
		log.Println("http proxy listening on", server.Addr)
		if err := server.ListenAndServe(); err != nil {
			log.Printf("failed to serve http: %v", err)
			return
		}
	}()

	return server
}

func newHTTPProxyServer(cfg *config.ProtocolCfg, resolver routing.RegionResolver) *http.Server {
	mux := http.NewServeMux()
	httpHandler := forwarder.HTTP(cfg, resolver).Handler()
	mux.HandleFunc("/", httpHandler)
	addr := cfg.Listen
	if !strings.HasPrefix(cfg.Listen, ":") {
		addr = ":" + cfg.Listen
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
	}

	return server
}

func startGRPCProxy(cfg *config.ServiceCfg, resolver routing.RegionResolver) *grpc.Server {
	if cfg.GRPC == nil {
		log.Println("gRPC proxy not enabled")
		return nil
	}

	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", ":"+cfg.GRPC.Listen)
	if err != nil {
		log.Printf("failed to listen to %s: %v", cfg.GRPC.Listen, err)
		return nil
	}

	grpcHandler := forwarder.GRPC(cfg.GRPC, resolver).Handler()
	server := grpc.NewServer(grpc.UnknownServiceHandler(grpcHandler), grpc.ForceServerCodec(&codec.PassThrough{}))
	go func() {
		log.Println("gRPC proxy listening on", lis.Addr().String())
		if err := server.Serve(lis); err != nil {
			log.Printf("failed to serve grpc: %v", err)
			return
		}
	}()

	return server
}
