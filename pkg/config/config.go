package config

import (
	"gopkg.in/yaml.v3"
	"os"
)

type Common struct {
	Listen       string         `yaml:"listen"`
	Destinations []*Destination `yaml:"destinations"`
}
type Destination struct {
	Entrypoint string `yaml:"entrypoint"`
	EU         string `yaml:"eu"`
	US         string `yaml:"us"`
}

type DataResolver struct {
	Type       string          `yaml:"type"`
	Endpoint   string          `yaml:"endpoint"`
	Method     string          `yaml:"method"`
	QueryParam string          `yaml:"query_param"`
	Static     string          `yaml:"static"`
	RegionMap  *RegionResolver `yaml:"region_resolver"`
}

type RegionResolver struct {
	Type    string            `yaml:"type"`
	Field   string            `yaml:"field"`
	Mapping map[string]string `yaml:"mapping"`
}

type ServiceCfg struct {
	HTTP          *Common       `yaml:"http"`
	GRPC          *Common       `yaml:"grpc"`
	DataRetriever *DataResolver `yaml:"data_retriever"`
}

func Load(path string) (*ServiceCfg, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ServiceCfg
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
