package routing_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CanobbioE/poly-route/internal/config"
	"github.com/CanobbioE/poly-route/internal/routing"
)

func TestHTTPResolver(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var key string
		cfgKey := "user_id"
		key = r.URL.Query().Get(cfgKey)

		resp := map[string]any{"info": map[string]any{"loc": map[string]any{"short": key}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	retr := &config.RegionRetriever{
		Type:       config.RegionResolverTypeHTTP,
		URL:        srv.URL,
		Method:     http.MethodGet,
		QueryParam: "user_id",
		RegionResolver: &config.RegionResolver{
			Type:    "mapping",
			Field:   "info.loc.short",
			Mapping: map[string]string{"alice": "region-A"},
		},
	}

	rslv, err := routing.NewResolver(retr)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}

	region, err := rslv.ResolveRegion(context.Background(), "alice")
	if err != nil {
		t.Fatalf("unexpected error resolving region: %v", err)
	}
	if region != "region-A" {
		t.Fatalf("unexpected region: %s", region)
	}
}
