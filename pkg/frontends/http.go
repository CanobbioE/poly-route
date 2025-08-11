package frontends

import (
	"github.com/CanobbioE/poly-route/pkg/config"
	"github.com/CanobbioE/poly-route/pkg/routing"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func findHTTPBackend(cfg *config.ServiceCfg, path string, region string) (string, bool) {
	for _, d := range cfg.HTTP.Destinations {
		if strings.HasPrefix(path, d.Entrypoint) {
			switch region {
			case "eu":
				return d.EU, true
			case "us":
				return d.US, true
			default:
				return "", false
			}
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
		// keep original path (backend should accept full path); you can strip prefix if needed
		req.URL.Path = origPath
		req.URL.RawQuery = r.URL.RawQuery
		req.Header = r.Header
		req.Host = u.Host
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		return nil
	}
	proxy.ServeHTTP(w, r)
}

// HTTP frontend handler
func HTTP(cfg *config.ServiceCfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Header.Get("X-User")
		if user == "" {
			user = r.URL.Query().Get("user")
		}
		if user == "" {
			http.Error(w, "missing region (set X-User header or ?user=)", http.StatusBadRequest)
			return
		}

		region, err := routing.GetRegion(cfg, user)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

		target, ok := findHTTPBackend(cfg, r.URL.Path, region)
		if !ok {
			http.Error(w, "no backend for this path/region", http.StatusBadGateway)
			return
		}
		httpForward(target, w, r)
	}
}
