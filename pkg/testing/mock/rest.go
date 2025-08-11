package mock

import (
	"errors"
	"fmt"
	"log"
	"net/http"
)

func startMockHTTPBackend(addr, name string) {
	h := http.NewServeMux()
	h.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprintf(w, "%s backend (%s) got path=%s\n", name, addr, r.URL.Path)
	})
	server := &http.Server{Addr: addr, Handler: h}
	log.Printf("mock http backend %s listening on %s", name, addr)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("mock http backend failed: %v", err)
		}
	}()
}
