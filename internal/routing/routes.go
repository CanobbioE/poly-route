package routing

import (
	"slices"
	"strings"

	"github.com/CanobbioE/poly-route/internal/config"
)

type routeKind int

const (
	// RouteExact indicates that the full route path must be matched exactly.
	RouteExact routeKind = iota

	// RoutePrefix indicates that just the route's prefix must be matched (e.g. "/api/v1/*").
	RoutePrefix

	// RouteMatchAll is a route matching wildcard, all routes will be matched (e.g. "*" or "/*").
	RouteMatchAll
)

// CompiledRoute represents a route that has been pre-compiled for faster lookup.
type CompiledRoute struct {
	Mappings map[string]string
	Prefix   string
	Kind     routeKind
}

// CompileRoutes returns a slice of CompiledRoute generated from the destinations defined in cfg.
func CompileRoutes(cfg *config.ProtocolCfg, p config.Protocol) []*CompiledRoute {
	var exact, prefix, matchAll []*CompiledRoute

	for key, mappings := range cfg.Destinations {
		r := &CompiledRoute{Mappings: mappings}
		switch {
		case key == "*" || key == "/*":
			r.Kind = RouteMatchAll
			matchAll = append(matchAll, r)
		case strings.HasSuffix(key, "/*"):
			r.Kind = RoutePrefix
			r.Prefix = key[:len(key)-2]
			if p == config.ProtocolGRPC {
				// strip "*", keep trailing "/"
				r.Prefix = key[:len(key)-1]
			}
			prefix = append(prefix, r)
		default:
			r.Kind = RouteExact
			r.Prefix = key
			exact = append(exact, r)
		}
	}

	// sort descending by prefix length.
	slices.SortFunc(prefix, func(a, b *CompiledRoute) int {
		return len(b.Prefix) - len(a.Prefix)
	})

	return append(append(exact, prefix...), matchAll...)
}
