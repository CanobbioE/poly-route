package routing

import (
	"fmt"
	"github.com/CanobbioE/poly-route/pkg/config"
	"io"
	"net/http"
	"net/url"
)

func GetRegion(cfg *config.ServiceCfg, userID string) (string, error) {
	if cfg.DataRetriever.Type == "static" {
		return cfg.DataRetriever.Static, nil
	}
	if cfg.DataRetriever.Type != "http" {
		return "", fmt.Errorf("unsupported data retriever: %s", cfg.DataRetriever.Type)
	}

	u, _ := url.Parse(cfg.DataRetriever.Endpoint)
	q := u.Query()
	q.Set(cfg.DataRetriever.QueryParam, userID)
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// Minimal fake parser: expects response like {"country":"IT"}
	// TODO: what if it's nested?
	// TODO: handle http
	var country string
	fmt.Sscanf(string(body), `{"country":"%s"}`, &country)
	country = trimJSON(country)

	region, ok := cfg.DataRetriever.RegionMap.Mapping[country]
	if !ok {
		return "", fmt.Errorf("no mapping for country: %s", country)
	}
	return region, nil
}

func trimJSON(s string) string {
	if len(s) > 0 && s[len(s)-1] == '"' {
		return s[:len(s)-1]
	}
	return s
}
