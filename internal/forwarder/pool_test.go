package forwarder_test

import (
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/CanobbioE/poly-route/internal/forwarder"
)

// dialOpts used by all pool tests — insecure, no actual backend required
// because grpc.NewClient is non-blocking (it dials lazily).
var testDialOpts = []grpc.DialOption{
	grpc.WithTransportCredentials(insecure.NewCredentials()),
}

func TestConnPool_Get(t *testing.T) {
	t.Run("returns same connection", func(t *testing.T) {
		pool := forwarder.NewConnectionPool(testDialOpts...)

		c1, err := pool.Get("localhost:19999")
		if err != nil {
			t.Fatalf("first get: %v", err)
		}
		c2, err := pool.Get("localhost:19999")
		if err != nil {
			t.Fatalf("second get: %v", err)
		}
		if c1 != c2 {
			t.Error("expected same *ClientConn to be returned on repeated gets")
		}
	})

	t.Run("replaces shutdown connection", func(t *testing.T) {
		pool := forwarder.NewConnectionPool(testDialOpts...)

		c1, err := pool.Get("localhost:19998")
		if err != nil {
			t.Fatalf("first get: %v", err)
		}

		// force shutdown state
		if err = c1.Close(); err != nil {
			t.Fatalf("first close: %v", err)
		}

		c2, err := pool.Get("localhost:19998")
		if err != nil {
			t.Fatalf("get after shutdown: %v", err)
		}
		if c1 == c2 {
			t.Error("expected a new *ClientConn after the previous one was shut down")
		}
		if c2.GetState() == connectivity.Shutdown {
			t.Error("replacement connection should not be shut down immediately")
		}
	})

	t.Run("returns different connection", func(t *testing.T) {
		pool := forwarder.NewConnectionPool(testDialOpts...)

		c1, err := pool.Get("localhost:19997")
		if err != nil {
			t.Fatalf("get addr1: %v", err)
		}
		c2, err := pool.Get("localhost:19996")
		if err != nil {
			t.Fatalf("get addr2: %v", err)
		}
		if c1 == c2 {
			t.Error("different addresses should produce different connections")
		}
	})

	t.Run("concurrent same address", func(t *testing.T) {
		pool := forwarder.NewConnectionPool(testDialOpts...)

		const goroutines = 50
		conns := make([]*grpc.ClientConn, goroutines)
		var wg sync.WaitGroup
		wg.Add(goroutines)

		for i := range goroutines {
			go func(idx int) {
				defer wg.Done()
				c, err := pool.Get("localhost:19995")
				if err != nil {
					t.Errorf("goroutine %d: %v", idx, err)
					return
				}
				conns[idx] = c
			}(i)
		}
		wg.Wait()

		first := conns[0]
		for i, c := range conns {
			if c == nil {
				t.Errorf("goroutine %d: got nil conn", i)
				continue
			}
			if c != first {
				t.Errorf("goroutine %d: expected same conn as goroutine 0, got different pointer", i)
			}
		}
	})
}

func TestConnPool_CloseAll(t *testing.T) {
	pool := forwarder.NewConnectionPool(testDialOpts...)

	addrs := []string{"localhost:19994", "localhost:19993", "localhost:19992"}
	for _, addr := range addrs {
		if _, err := pool.Get(addr); err != nil {
			t.Fatalf("get %s: %v", addr, err)
		}
	}

	if err := pool.CloseAll(); err != nil {
		t.Fatalf("CloseAll: %v", err)
	}

	// after CloseAll, getting a connection should succeed (pool dials fresh ones).
	c, err := pool.Get(addrs[0])
	if err != nil {
		t.Fatalf("get after CloseAll: %v", err)
	}
	if c.GetState() == connectivity.Shutdown {
		t.Error("new connection after CloseAll should not be shut down")
	}
	_ = pool.CloseAll()
}
