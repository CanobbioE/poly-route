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
	routes         []*routing.CompiledRoute
}

// HTTP creates a new HTTPForwarder.
func HTTP(cfg *config.ProtocolCfg, resolver routing.RegionResolver, l logger.LazyLogger) *HTTPForwarder {
	fwd := &HTTPForwarder{
		cfg:            cfg,
		regionResolver: resolver,
		log:            l,
		routes:         routing.CompileRoutes(cfg, config.ProtocolHTTP),
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
			x.log.Error("region resolver failed", "error", err)
			http.Error(w, "failed to resolve region", http.StatusBadRequest)
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
		x.proxy.ServeHTTP(w, r.WithContext(ctx))
	}
}

// FindBackend finds an HTTP backend by best match using the HTTPForwarder protocol configuration.
// The best route is either an exact match with the entrypoint or the longest wild-card-suffixed match.
func (x *HTTPForwarder) FindBackend(entrypoint, region string) (string, bool) {
	for _, r := range x.routes {
		switch r.Kind {
		case routing.RouteExact:
			if r.Prefix != entrypoint {
				continue
			}
		case routing.RoutePrefix:
			// accept either exact equality or prefix + '/'
			if entrypoint != r.Prefix && !strings.HasPrefix(entrypoint, r.Prefix+"/") {
				continue
			}
		case routing.RouteMatchAll:
			// always matches
		}

		dest, ok := r.Mappings[region]
		if !ok {
			return "", false
		}

		if r.Kind == routing.RouteExact {
			return dest, true
		}

		// for prefix matches, append the suffix only if present
		suffix := entrypoint[len(r.Prefix):]
		if r.Kind == routing.RouteMatchAll {
			suffix = entrypoint
		}
		u, err := url.JoinPath(dest, suffix)
		return u, err == nil
	}
	return "", false
}
