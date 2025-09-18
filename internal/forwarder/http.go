package forwarder

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/CanobbioE/poly-route/internal/config"
	"github.com/CanobbioE/poly-route/internal/routing"
)

const (
	// HeaderRegionKey is the header key used to retrieve the region from the api call.
	HeaderRegionKey = "X-Poly-Route-Region"
	// QueryParamRegionKey is the query parameter key used to retrieve the region from the api call.
	// Used only if HeaderRegionKey was not specified.
	QueryParamRegionKey = "region"
)

// HTTPForwarder implements a reverse transport proxy for HTTP.
type HTTPForwarder struct {
	cfg            *config.ProtocolCfg
	regionResolver routing.RegionResolver
}

// HTTP creates a new HTTPForwarder.
func HTTP(cfg *config.ProtocolCfg, resolver routing.RegionResolver) *HTTPForwarder {
	return &HTTPForwarder{
		cfg:            cfg,
		regionResolver: resolver,
	}
}

// Handler returns a [http.HandlerFunc] that uses a [httputil.ReverseProxy] to forward the incoming request.
func (x *HTTPForwarder) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		region := r.Header.Get(HeaderRegionKey)
		if region == "" {
			region = r.URL.Query().Get(QueryParamRegionKey)
		}
		if region == "" {
			http.Error(w, "missing region (set "+HeaderRegionKey+" header or ?"+QueryParamRegionKey+"=)", http.StatusBadRequest)
			return
		}

		resolvedRegion, err := x.regionResolver.ResolveRegion(r.Context(), region)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		target, ok := x.FindBackend(r.URL.Path, resolvedRegion)
		if !ok {
			http.Error(w, "no backend for this path/region", http.StatusBadGateway)
			return
		}
		httpForward(target, w, r)
	}
}

// FindBackend finds an HTTP backend by best match using the HTTPForwarder protocol configuration.
// The best route is either an exact match with the entrypoint or the longest wild-card-suffixed match.
func (x *HTTPForwarder) FindBackend(entrypoint, region string) (string, bool) {
	// exact match
	mappings, exactMatch := x.cfg.Destinations[entrypoint]
	if exactMatch {
		v, ok := mappings[region]
		return v, ok
	}

	var bestMatch string
	for key := range x.cfg.Destinations {
		switch {
		case key == "*":
			// match-all wildcard
			bestMatch = key
			continue

		case !strings.HasSuffix(key, "/*"):
			// no wildcard, skip
			continue
		case strings.HasPrefix(entrypoint, key[:len(key)-1]) || strings.HasPrefix(entrypoint, key[:len(key)-2]):
			// key has wildcard /*
			// and what comes before * or /* is part of the entrypoint
			if len(key) <= len(bestMatch) {
				continue // a better match already exists
			}
			entrypoint = entrypoint[len(key)-1:]
			bestMatch = key
		}
	}

	mappings = x.cfg.Destinations[bestMatch]
	v, ok := mappings[region]
	if !ok {
		return "", false
	}

	u, err := url.JoinPath(v, entrypoint)
	return u, err == nil
}

func httpForward(target string, w http.ResponseWriter, r *http.Request) {
	log.Printf("forwarding HTTP request towards addr=%s method=%s", target, r.Method)
	u, err := url.Parse(target)
	if err != nil {
		http.Error(w, "bad backend url", http.StatusInternalServerError)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = u.Scheme
		req.URL.Host = u.Host
		req.URL.Path = u.Path
		req.URL.RawQuery = r.URL.RawQuery
		// TODO: consider adding X-Forwarded-For
		req.Header = r.Header
		req.Host = u.Host
	}
	proxy.ModifyResponse = func(_ *http.Response) error {
		return nil
	}
	proxy.ServeHTTP(w, r)
}
