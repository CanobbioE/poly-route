package forwarder

import (
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

func findHTTPBackend(cfg *config.ServiceCfg, path, region string) (string, bool) {
	for _, d := range cfg.HTTP.Destinations {
		if strings.HasPrefix(path, d.Entrypoint) {
			v, ok := d.Mapping[region]
			return v, ok
		}
	}
	return "", false
}

func httpForward(target string, w http.ResponseWriter, r *http.Request) {
	u, err := url.Parse(target)
	if err != nil {
		http.Error(w, "bad backend url", http.StatusInternalServerError)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	origPath := r.URL.Path
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = u.Scheme
		req.URL.Host = u.Host
		req.URL.Path = origPath
		req.URL.RawQuery = r.URL.RawQuery
		req.Header = r.Header
		req.Host = u.Host
	}
	proxy.ModifyResponse = func(_ *http.Response) error {
		return nil
	}
	proxy.ServeHTTP(w, r)
}

// HTTP frontend handler.
// TODO: make a struct and add dependencies.
func HTTP(cfg *config.ServiceCfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		region := r.Header.Get(HeaderRegionKey)
		if region == "" {
			region = r.URL.Query().Get(QueryParamRegionKey)
		}
		if region == "" {
			http.Error(w, "missing region (set "+HeaderRegionKey+" header or ?"+QueryParamRegionKey+"=)", http.StatusBadRequest)
			return
		}

		// TODO: dependency
		resolver, err := routing.NewResolver(cfg.RegionRetriever)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resolvedRegion, err := resolver.ResolveRegion(r.Context(), region)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		target, ok := findHTTPBackend(cfg, r.URL.Path, resolvedRegion)
		if !ok {
			http.Error(w, "no backend for this path/region", http.StatusBadGateway)
			return
		}
		httpForward(target, w, r)
	}
}
