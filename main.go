package main

import (
	"context"
	"log/slog"
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
	"github.com/CanobbioE/poly-route/internal/logger"
	"github.com/CanobbioE/poly-route/internal/routing"
)

func main() {
	log := logger.NewSlog(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfgPath := os.Getenv("CONFIG_FILE_PATH")
	if cfgPath == "" {
		log.Info("couldn't read configuration filepath as env variable, please set CONFIG_FILE_PATH")
		log.Info("defaulting to config.yaml")
		cfgPath = "config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Error("could not read config file", "path", cfgPath, "error", err)
		return
	}

	regionResolver, err := routing.NewResolver(
		cfg.RegionRetriever,
		routing.WithHTTPClient(&http.Client{Timeout: 3 * time.Second}),
	)
	if err != nil {
		log.Error("failed to create region resolver", "error", err)
		return
	}

	httpProxy := startHTTPProxy(cfg, regionResolver, log)
	grpcProxy, grpcForwarder := startGRPCProxy(cfg, regionResolver, log)
	defer func() {
		if grpcForwarder != nil {
			// closing the forwarder as last thing ensures no connection
			// from the pool is used after it has been closed
			_ = grpcForwarder.Close()
		}
	}()
	graphQLProxy := startGraphQLProxy(cfg, regionResolver, log)

	if httpProxy == nil && grpcProxy == nil && graphQLProxy == nil {
		log.Error("no proxy server set up, stopping now")
		return
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Info("shutting down proxies...")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if httpProxy != nil {
		if err = httpProxy.Shutdown(ctx); err != nil {
			log.Warn("failed to shutdown proxy HTTP server")
		}
	}
	if grpcProxy != nil {
		grpcProxy.GracefulStop()
	}
	if graphQLProxy != nil {
		if err = graphQLProxy.Shutdown(ctx); err != nil {
			log.Warn("failed to shutdown proxy GraphQL server")
		}
	}
}

func startGraphQLProxy(cfg *config.ServiceCfg, resolver routing.RegionResolver, l logger.LazyLogger) *http.Server {
	if cfg.GraphQL == nil {
		l.Info("GraphQL proxy not enabled")
		return nil
	}

	server := newHTTPProxyServer(cfg.GraphQL, resolver, l)

	go func() {
		l.Info("graphql proxy is listening", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil {
			l.Error("failed to serve graphql over http", "error", err)
			return
		}
	}()

	return server
}

func startHTTPProxy(cfg *config.ServiceCfg, resolver routing.RegionResolver, l logger.LazyLogger) *http.Server {
	if cfg.HTTP == nil {
		l.Info("HTTP proxy not enabled")
		return nil
	}

	server := newHTTPProxyServer(cfg.HTTP, resolver, l)

	go func() {
		l.Info("http proxy is listening", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil {
			l.Error("failed to serve http", "error", err)
			return
		}
	}()

	return server
}

func newHTTPProxyServer(cfg *config.ProtocolCfg, resolver routing.RegionResolver, l logger.LazyLogger) *http.Server {
	mux := http.NewServeMux()
	httpHandler := forwarder.HTTP(cfg, resolver, l).Handler()
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

// startGRPCProxy returns both the gRPC server and the forwarder so that the
// caller can close the connection pool during shutdown.
func startGRPCProxy(
	cfg *config.ServiceCfg,
	resolver routing.RegionResolver,
	l logger.LazyLogger,
) (*grpc.Server, *forwarder.GRPCForwarder) {
	if cfg.GRPC == nil {
		l.Info("gRPC proxy not enabled")
		return nil, nil
	}

	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", ":"+cfg.GRPC.Listen)
	if err != nil {
		l.Error("failed to listen", "address", cfg.GRPC.Listen, "error", err)
		return nil, nil
	}

	grpcFwd := forwarder.GRPC(cfg.GRPC, resolver, l)
	server := grpc.NewServer(
		grpc.UnknownServiceHandler(grpcFwd.Handler()),
		grpc.ForceServerCodec(&codec.PassThrough{}),
	)

	go func() {
		l.Info("gRPC proxy is listening", "address", lis.Addr().String())
		if err := server.Serve(lis); err != nil {
			l.Error("failed to serve grpc", "error", err)
			return
		}
	}()

	return server, grpcFwd
}
