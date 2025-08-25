package mock

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/graphql-go/graphql"
)

// StartMockGraphQLBackend is a fire-and-forget function that starts a simple GraphQL over HTTP server.
// // The server accepts POST requests on the endpoint "/graphql".
// // The response is a JSON with the server info.
func StartMockGraphQLBackend(addr, name string) *http.Server {
	h := http.NewServeMux()
	schema, err := newSchema(addr, name)
	if err != nil {
		log.Fatal(err) //nolint:revive // test files can deep-exit
	}
	h.HandleFunc("/graphql", graphqlHandler(schema))

	server := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 30 * time.Second}

	go func() {
		log.Printf("mock graphql backend %s listening on %s", name, addr)
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("failed to start server: %v", err) //nolint:revive // test files can deep-exit
		}
	}()

	return server
}

func newSchema(addr, name string) (*graphql.Schema, error) {
	fields := graphql.Fields{
		"hello": &graphql.Field{
			Type: graphql.String,
			Resolve: func(_ graphql.ResolveParams) (any, error) {
				return `{"backend": ` + name + `, "addr": ` + addr + `, "path": "graphql/hello"}`, nil
			},
		},
	}

	rootQuery := graphql.ObjectConfig{Name: "RootQuery", Fields: fields}
	schemaConfig := graphql.SchemaConfig{Query: graphql.NewObject(rootQuery)}
	schema, err := graphql.NewSchema(schemaConfig)
	if err != nil {
		return &schema, fmt.Errorf("failed to create schema: %w", err)
	}
	return &schema, nil
}

func graphqlHandler(schema *graphql.Schema) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var params struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			http.Error(w, "failed to decode request", http.StatusBadRequest)
			return
		}

		result := graphql.Do(graphql.Params{
			Schema:        *schema,
			RequestString: params.Query,
		})

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			http.Error(w, "failed to encode response", http.StatusBadRequest)
			return
		}
	}
}
