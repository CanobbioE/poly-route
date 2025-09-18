package forwarder_test

import (
	"testing"

	"github.com/CanobbioE/poly-route/internal/config"
	"github.com/CanobbioE/poly-route/internal/forwarder"
)

func TestHTTPForwarder_FindBackend(t *testing.T) {
	type args struct {
		cfg        *config.ProtocolCfg
		entrypoint string
		region     string
	}
	tests := []struct {
		name  string
		args  args
		want  string
		want1 bool
	}{
		{
			name: "match-all wildcard",
			args: args{
				cfg: &config.ProtocolCfg{
					Destinations: map[string]map[string]string{
						"*": {"region": "http://localhost:8080/redirect"},
					},
				},
				entrypoint: "/test/v1/config",
				region:     "region",
			},
			want:  "http://localhost:8080/redirect/test/v1/config",
			want1: true,
		},
		{
			name: "match-all wildcard with slash",
			args: args{
				cfg: &config.ProtocolCfg{
					Destinations: map[string]map[string]string{
						"/*": {"region": "http://localhost:8080/redirect"},
					},
				},
				entrypoint: "/test/v1/config",
				region:     "region",
			},
			want:  "http://localhost:8080/redirect/test/v1/config",
			want1: true,
		},
		{
			name: "exact match",
			args: args{
				cfg: &config.ProtocolCfg{
					Destinations: map[string]map[string]string{
						"/test/v1/config": {"region": "http://localhost:8080/redirect"},
					},
				},
				entrypoint: "/test/v1/config",
				region:     "region",
			},
			want:  "http://localhost:8080/redirect",
			want1: true,
		},
		{
			name: "partial wildcard match",
			args: args{
				cfg: &config.ProtocolCfg{
					Destinations: map[string]map[string]string{
						"/test/*": {"region": "http://localhost:8080/redirect"},
					},
				},
				entrypoint: "/test/v1/config",
				region:     "region",
			},
			want:  "http://localhost:8080/redirect/v1/config",
			want1: true,
		},
		{
			name: "partial wildcard match trailing slash",
			args: args{
				cfg: &config.ProtocolCfg{
					Destinations: map[string]map[string]string{
						"/test/*": {"region": "http://localhost:8080/redirect"},
					},
				},
				entrypoint: "/test/v1/config",
				region:     "region",
			},
			want:  "http://localhost:8080/redirect/v1/config",
			want1: true,
		},
		{
			name: "best match",
			args: args{
				cfg: &config.ProtocolCfg{
					Destinations: map[string]map[string]string{
						"*":               {"region": "http://localhost"},
						"/test/*":         {"region": "http://localhost"},
						"/test/v1/*":      {"region": "http://localhost"},
						"/test/v1/config": {"region": "http://localhost:8080/redirect"},
					},
				},
				entrypoint: "/test/v1/config",
				region:     "region",
			},
			want:  "http://localhost:8080/redirect",
			want1: true,
		},
		{
			name: "no match",
			args: args{
				cfg: &config.ProtocolCfg{
					Destinations: map[string]map[string]string{
						"/test/v1/config": {"region": "http://localhost:8080/redirect"},
					},
				},
				entrypoint: "/test/v2/config",
				region:     "region",
			},
			want:  "",
			want1: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ht := forwarder.HTTP(tt.args.cfg, nil)
			got, got1 := ht.FindBackend(tt.args.entrypoint, tt.args.region)
			if got != tt.want {
				t.Errorf("http.FindBackend() got = '%v', want '%v'", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("http.FindBackend() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
