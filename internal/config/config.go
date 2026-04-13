package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Protocol is used to identify which protocol a ProtocolCfg defines.
type Protocol string

const (
	// ProtocolHTTP identifies the HTTP protocol.
	ProtocolHTTP Protocol = "http"
	// ProtocolGRPC identifies the GRPC protocol.
	ProtocolGRPC Protocol = "grpc"
	// ProtocolGraphQL identifies the GraphQL protocol.
	ProtocolGraphQL Protocol = "graphql"

	// RegionResolverTypeHTTP identifies the HTTP region resolver.
	RegionResolverTypeHTTP = "http"
	// RegionResolverTypeHStatic identifies the static region resolver.
	RegionResolverTypeHStatic = "static"
)

// ProtocolCfg configures the incoming and outgoing proxy requests for a Protocol.
type ProtocolCfg struct {
	Destinations map[string]map[string]string `yaml:"destinations"`
	Listen       string                       `yaml:"listen"`
}

// RegionRetriever configures how and from where the region value should be retrieved.
type RegionRetriever struct {
	RegionResolver *RegionResolver `yaml:"region_resolver"`
	Type           string          `yaml:"type"`
	URL            string          `yaml:"url"`
	Method         string          `yaml:"method"`
	QueryParam     string          `yaml:"query_param"`
	Static         string          `yaml:"static"`
}

// RegionResolver configures how to resolve the region value retrieved from the RegionRetriever.
type RegionResolver struct {
	Mapping map[string]string `yaml:"mapping"`
	Type    string            `yaml:"type"`
	Field   string            `yaml:"field"`
}

// ServiceCfg is the whole service configuration.
type ServiceCfg struct {
	HTTP            *ProtocolCfg     `yaml:"http"`
	GRPC            *ProtocolCfg     `yaml:"grpc"`
	GraphQL         *ProtocolCfg     `yaml:"graphql"`
	RegionRetriever *RegionRetriever `yaml:"region_retriever"`
}

// Load the ServiceCfg from the given file path and validates it before returning.
func Load(path string) (*ServiceCfg, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var cfg ServiceCfg
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	return &cfg, nil
}

// Validate checks the ServiceCfg for consistency and pre-parses all URLs so that
// misconfigured backends are caught at startup rather than at request time.
func (c *ServiceCfg) Validate() error {
	if c.HTTP == nil && c.GRPC == nil && c.GraphQL == nil {
		return errors.New("at least one Protocol (http, grpc, graphql) must be configured")
	}

	if err := c.RegionRetriever.validate(); err != nil {
		return fmt.Errorf("region_retriever: %w", err)
	}

	if err := c.HTTP.validate(ProtocolHTTP); err != nil {
		return err
	}
	if err := c.GraphQL.validate(ProtocolGraphQL); err != nil {
		return err
	}

	return c.GRPC.validate(ProtocolGRPC)
}

func (r *RegionRetriever) validate() error {
	if r == nil {
		return errors.New("region_retriever must be defined")
	}

	switch r.Type {
	case RegionResolverTypeHTTP:
		if r.URL == "" {
			return errors.New("url is required for \"" + RegionResolverTypeHTTP + "\" retriever")
		}
		if _, err := url.ParseRequestURI(r.URL); err != nil {
			return fmt.Errorf("url %q is not a valid URL: %w", r.URL, err)
		}
		if r.Method == "" {
			return errors.New("method is required for http retriever")
		}
		if r.QueryParam == "" {
			return errors.New("query_param is required for http retriever")
		}
		if err := r.RegionResolver.validate(); err != nil {
			return fmt.Errorf("region_resolver: %w", err)
		}
	case RegionResolverTypeHStatic:
		if r.Static == "" {
			return errors.New("static value must be defined when type is \"" + RegionResolverTypeHStatic + "\"")
		}
	default:
		return errors.New("unknown type \"" + r.Type + "\", must be \"" + RegionResolverTypeHTTP +
			"\" or \"" + RegionResolverTypeHStatic + "\"")
	}

	return nil
}

func (r *RegionResolver) validate() error {
	if r == nil {
		return errors.New("region_resolver must be defined for http retriever")
	}
	if r.Type == "" {
		return errors.New("type must be defined")
	}
	if r.Field == "" {
		return errors.New("field must be defined")
	}
	if len(r.Mapping) == 0 {
		return errors.New("mapping must have at least one entry")
	}
	return nil
}

// validate checks a single Protocol config.
// - http/graphql backends (must start with http:// or https://)
// - gRPC backends (host:port format, no scheme).
func (cfg *ProtocolCfg) validate(p Protocol) error {
	if cfg == nil {
		return nil
	}
	if cfg.Listen == "" {
		return errors.New(string(p) + ": listen port must not be empty")
	}

	if len(cfg.Destinations) == 0 {
		return errors.New(string(p) + ": destinations must not be empty")
	}

	for route, regionMap := range cfg.Destinations {
		if len(regionMap) == 0 {
			return errors.New(string(p) + ": route \"" + route + "\" has no region mappings")
		}

		for region, addr := range regionMap {
			if addr == "" {
				return errors.New(string(p) + ": route \"" + route + "\" region \"" + region + "\" has an empty address")
			}

			var err error
			switch p {
			case ProtocolHTTP, ProtocolGraphQL:
				err = validateHTTPAddress(p, route, region, addr)
			case ProtocolGRPC:
				err = validateGRPCAddress(p, route, region, addr)
			}

			if err != nil {
				return err
			}
		}
	}

	return nil
}

func validateHTTPAddress(p Protocol, route, region, addr string) error {
	u, err := url.ParseRequestURI(addr)
	if err != nil {
		return fmt.Errorf("%s: route %q region %q: %q is not a valid URL: %w", p, route, region, addr, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New(string(p) + ": route \"" + route + "\" region \"" +
			region + "\": \"" + addr + "\" must use http or https scheme")
	}
	if u.Host == "" {
		return errors.New(string(p) + ": route \"" + route + "\" region \"" + region + "\": \"" + addr + "\" has no host")
	}
	return nil
}

func validateGRPCAddress(p Protocol, route, region, addr string) error {
	// Strip any path component that may be legitimately included in gRPC
	// wildcard destination addresses (e.g. "localhost:9095/mockserver.v1.MockService").
	// We only validate the host:port portion.
	hostPort := addr
	if idx := strings.Index(addr, "/"); idx != -1 {
		hostPort = addr[:idx]
	}
	if hostPort == "" {
		return errors.New(string(p) + ": route \"" + route + "\" region \"" + region + "\": \"" + addr + "\" has no host")
	}
	if strings.Contains(hostPort, "://") {
		return errors.New(string(p) + ": route \"" + route + "\" region \"" + region +
			"\": gRPC address \"" + addr + "\" must not contain a scheme")
	}
	return nil
}
