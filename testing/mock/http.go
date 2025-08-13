package mock

import (
	"errors"
	"log"
	"net/http"
	"time"
)

// StartMockHTTPBackend is a fire-and-forget function that starts a simple HTTP server.
// The server accepts any method on the endpoint "/".
// The response is a JSON with the server info.
func StartMockHTTPBackend(addr, name string) *http.Server {
	h := http.NewServeMux()
	h.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"backend": ` + name + `, "addr": ` + addr + `, "path": ` + r.Method + r.URL.Path + `}`))
	})
	server := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 30 * time.Second}
	log.Printf("mock http backend %s listening on %s", name, addr)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("mock http backend failed to serve: %v", err) //nolint:revive // test files can deep-exit
		}
	}()
	return server
}
