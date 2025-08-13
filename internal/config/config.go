package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ProtocolCfg configures the incoming and outgoing proxy requests for a protocol.
type ProtocolCfg struct {
	Listen       string         `yaml:"listen"`
	Destinations []*Destination `yaml:"destinations"`
}

// Destination is a protocol agnostic target for the proxy.
type Destination struct {
	Mapping    map[string]string `yaml:"mapping"`
	Entrypoint string            `yaml:"entrypoint"`
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

// RegionResolver configures  how to resolve the region value retrieved from the RegionRetriever.
type RegionResolver struct {
	Mapping map[string]string `yaml:"mapping"`
	Type    string            `yaml:"type"`
	Field   string            `yaml:"field"`
}

// ServiceCfg is the whole service configuration.
type ServiceCfg struct {
	HTTP            *ProtocolCfg     `yaml:"http"`
	GRPC            *ProtocolCfg     `yaml:"grpc"`
	RegionRetriever *RegionRetriever `yaml:"region_retriever"`
}

// Load the ServiceCfg from the given file path.
func Load(path string) (*ServiceCfg, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var cfg ServiceCfg
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
