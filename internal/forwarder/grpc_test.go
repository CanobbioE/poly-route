package forwarder_test

import (
	"testing"

	"github.com/CanobbioE/poly-route/internal/config"
	"github.com/CanobbioE/poly-route/internal/forwarder"
)

func TestGRPCForwarder_FindBackend(t *testing.T) {
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
			name: "match-all wildcard match",
			args: args{
				cfg: &config.ProtocolCfg{
					Destinations: map[string]map[string]string{
						"*": {"region": "localhost:8080"},
					},
				},
				entrypoint: "/mockserver.v1.MockService/Invoke",
				region:     "region",
			},
			want:  "localhost:8080/mockserver.v1.MockService/Invoke",
			want1: true,
		},
		{
			name: "partial wildcard match",
			args: args{
				cfg: &config.ProtocolCfg{
					Destinations: map[string]map[string]string{
						"/mockserver.v1.MockService/*": {"region": "localhost:8080/mockserver.v1.MockService"},
					},
				},
				entrypoint: "/mockserver.v1.MockService/Invoke",
				region:     "region",
			},
			want:  "localhost:8080/mockserver.v1.MockService/Invoke",
			want1: true,
		},
		{
			name: "partial wildcard match trailing slash",
			args: args{
				cfg: &config.ProtocolCfg{
					Destinations: map[string]map[string]string{
						"/mockserver.v1.MockService/*": {"region": "localhost:8080/mockserver.v1.MockService/"},
					},
				},
				entrypoint: "/mockserver.v1.MockService/Invoke",
				region:     "region",
			},
			want:  "localhost:8080/mockserver.v1.MockService/Invoke",
			want1: true,
		},
		{
			name: "exact match",
			args: args{
				cfg: &config.ProtocolCfg{
					Destinations: map[string]map[string]string{
						"/mockserver.v1.MockService/Invoke": {"region": "localhost:8080/mockserver.v1.MockService/Invoke"},
					},
				},
				entrypoint: "/mockserver.v1.MockService/Invoke",
				region:     "region",
			},
			want:  "localhost:8080/mockserver.v1.MockService/Invoke",
			want1: true,
		},
		{
			name: "best match",
			args: args{
				cfg: &config.ProtocolCfg{
					Destinations: map[string]map[string]string{
						"*":                                 {"region": "localhost"},
						"/mockserver.v1.MockService/*":      {"region": "localhost/mockserver.v1.MockService/*"},
						"/mockserver.v1.MockService/Invoke": {"region": "localhost:8080/mockserver.v1.MockService/Invoke"},
					},
				},
				entrypoint: "/mockserver.v1.MockService/Invoke",
				region:     "region",
			},
			want:  "localhost:8080/mockserver.v1.MockService/Invoke",
			want1: true,
		},
		{
			name: "no match",
			args: args{
				cfg: &config.ProtocolCfg{
					Destinations: map[string]map[string]string{
						"/mockserver.v1.MockService/Other": {"region": "localhost:8080/mockserver.v1.MockService/*"},
					},
				},
				entrypoint: "/mockserver.v1.MockService/Invoke",
				region:     "region",
			},
			want:  "",
			want1: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := forwarder.GRPC(tt.args.cfg, nil)
			got, got1 := x.FindBackend(tt.args.entrypoint, tt.args.region)
			if got != tt.want {
				t.Errorf("grpc.FindBackend() got = '%v', want '%v'", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("grpc.FindBackend() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
