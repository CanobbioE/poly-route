package mock

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

// StartMockRegionRepository is a fire-and-forget function to start
// a simple region retriever server that accepts http requests.
// Call GET /userinfo with query parameters ?user_id=<region> to mock a region retrieval.
// The HTTP call returns the query parameter value as part of the JSON response in the format of:
// {"country": "<region>"}.
func StartMockRegionRepository(addr string) *http.Server {
	h := http.NewServeMux()
	h.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		qp := r.URL.Query().Get("user_id")
		_, _ = fmt.Fprintf(w, "%s", `{"country": "`+qp+`"}`)
	})
	server := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 30 * time.Second}
	log.Printf("mock http backend listening on %s", addr)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("mock http backend failed: %v", err) //nolint:revive // test files can deep-exit
		}
	}()
	return server
}
