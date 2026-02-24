package forwarder

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/CanobbioE/poly-route/internal/config"
	"github.com/CanobbioE/poly-route/internal/logger"
	"github.com/CanobbioE/poly-route/internal/routing"
)

type proxyKey string

const targetKey proxyKey = "proxy-target"

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
	log            logger.LazyLogger
	proxy          *httputil.ReverseProxy
}

// HTTP creates a new HTTPForwarder.
func HTTP(cfg *config.ProtocolCfg, resolver routing.RegionResolver, l logger.LazyLogger) *HTTPForwarder {
	fwd := &HTTPForwarder{
		cfg:            cfg,
		regionResolver: resolver,
		log:            l,
	}

	fwd.proxy = &httputil.ReverseProxy{
		Director: fwd.director,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			l.Error("http proxy error", "error", err, "url", r.URL.String())
			w.WriteHeader(http.StatusBadGateway)
		},
		Transport: http.DefaultTransport,
	}

	return fwd
}

func (*HTTPForwarder) director(req *http.Request) {
	target, ok := req.Context().Value(targetKey).(*url.URL)
	if !ok {
		return
	}

	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host
	req.URL.Path = target.Path
	req.Host = target.Host

	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		prior := req.Header.Get("X-Forwarded-For")
		if prior != "" {
			clientIP = prior + ", " + clientIP
		}
		req.Header.Set("X-Forwarded-For", clientIP)
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

		targetAddr, ok := x.FindBackend(r.URL.Path, resolvedRegion)
		if !ok {
			http.Error(w, "no backend found", http.StatusBadGateway)
			return
		}

		targetURL, err := url.Parse(targetAddr)
		if err != nil {
			http.Error(w, "invalid backend address", http.StatusInternalServerError)
			return
		}

		ctx := context.WithValue(r.Context(), targetKey, targetURL)
		// targetURL is resolved from a static configuration allow-list in x.cfg.Destinations.
		// This prevents arbitrary SSRF as only pre-defined backends are reachable.
		//
		//nolint:gosec // G704: Target URL is validated against internal routing config.
		x.proxy.ServeHTTP(w, r.WithContext(ctx))
	}
}

// FindBackend finds an HTTP backend by best match using the HTTPForwarder protocol configuration.
// The best route is either an exact match with the entrypoint or the longest wild-card-suffixed match.
//
// TODO: for a larger scale, use a Radix Tree for O(L) matching (where L is the path length).
// TODO: consider pre-computing paths at startup.
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
