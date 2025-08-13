package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/CanobbioE/poly-route/internal/config"
)

// RegionResolver is a generic resolver whose only job is to return the correct region based on input.
type RegionResolver interface {
	// ResolveRegion takes the input parameter and returns the mapped
	ResolveRegion(ctx context.Context, param string) (string, error)
}
type httpResolver struct {
	retrieverCfg *config.RegionRetriever
	resolverCfg  *config.RegionResolver
	client       *http.Client
}

type staticResolver struct {
	staticVal string
}

// ResolverOption used to create a new RegionResolver.
type ResolverOption interface {
	apply(r RegionResolver)
}

type withClient struct {
	Client *http.Client
}

func (w *withClient) apply(r RegionResolver) {
	if v, ok := r.(*httpResolver); ok {
		v.client = w.Client
	}
}

// WithHTTPClient specifies a custom HTTP client if the underlying resolver uses http.
// It does nothing otherwise.
func WithHTTPClient(client *http.Client) ResolverOption {
	return &withClient{client}
}

// NewResolver instantiates a new implementation of RegionResolver based on the value of [config.RegionRetriever.Type].
func NewResolver(cfg *config.RegionRetriever, opts ...ResolverOption) (RegionResolver, error) {
	switch cfg.Type {
	case "http":
		return newHTTPResolver(cfg, opts...)
	case "static":
		return newStaticResolver(cfg), nil
	default:
		return nil, fmt.Errorf("unknown resolver type: %s", cfg.Type)
	}
}

func newStaticResolver(cfg *config.RegionRetriever) RegionResolver {
	return &staticResolver{staticVal: cfg.Static}
}

func (x *staticResolver) ResolveRegion(_ context.Context, _ string) (string, error) {
	return x.staticVal, nil
}

func newHTTPResolver(cfg *config.RegionRetriever, options ...ResolverOption) (RegionResolver, error) {
	if cfg.Type != "http" {
		return nil, fmt.Errorf("unsupported data retriever: %s", cfg.Type)
	}

	r := &httpResolver{
		retrieverCfg: cfg,
		resolverCfg:  cfg.RegionResolver,
		client:       http.DefaultClient,
	}

	for _, option := range options {
		option.apply(r)
	}

	return r, nil
}

func (x *httpResolver) ResolveRegion(ctx context.Context, param string) (string, error) {
	var resp *http.Response

	switch x.retrieverCfg.Method {
	case http.MethodGet:
		u, err := url.Parse(x.retrieverCfg.URL)
		if err != nil {
			return "", fmt.Errorf("failed to parse retriever endpoint (%s): %w", x.retrieverCfg.URL, err)
		}
		q := u.Query()
		q.Set(x.retrieverCfg.QueryParam, param)
		u.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
		if err != nil {
			return "", err
		}
		resp, err = x.client.Do(req)
		if err != nil {
			return "", err
		}
		// TODO: handle post with body params
	default:
		return "", fmt.Errorf("unsupported data retriever method: %s", x.retrieverCfg.Method)
	}

	defer resp.Body.Close() //nolint:errcheck // test files skip error check for simplicity
	body, _ := io.ReadAll(resp.Body)

	responseJSON := map[string]any{}
	err := json.Unmarshal(body, &responseJSON)
	if err != nil {
		return "", fmt.Errorf("unmarshal response failed: %w", err)
	}

	return x.resolve(responseJSON)
}

func (x *httpResolver) resolve(m map[string]any) (string, error) {
	// TODO: handle nested keys like info.location.region.short_code
	regionKey, ok := m[x.resolverCfg.Field]
	if !ok {
		// TODO: handle `default` value: if specified, use that
		return "", fmt.Errorf("region not found at %s", x.resolverCfg.Field)
	}

	key, ok := regionKey.(string)
	if !ok {
		return "", fmt.Errorf("region at %s is not a string (%T)", x.resolverCfg.Field, regionKey)
	}

	region, ok := x.resolverCfg.Mapping[key]
	if !ok {
		return "", fmt.Errorf("no mapping specified for %s", key)
	}
	return region, nil
}
