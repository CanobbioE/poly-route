package forwarder

import (
	"errors"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// ConnectionPool caches gRPC client connections keyed by backend address.
// It is safe for concurrent use.
type ConnectionPool struct {
	conns    map[string]*grpc.ClientConn
	dialOpts []grpc.DialOption
	mu       sync.RWMutex
}

// NewConnectionPool creates a ConnectionPool.
// The provided dialOpts are reused for every new connection the pool dials.
func NewConnectionPool(opts ...grpc.DialOption) *ConnectionPool {
	return &ConnectionPool{
		conns:    make(map[string]*grpc.ClientConn),
		dialOpts: opts,
	}
}

// Get returns a healthy cached connection for addr, or dials a new one.
func (p *ConnectionPool) Get(addr string) (*grpc.ClientConn, error) {
	// an open connection already exists, return it
	p.mu.RLock()
	conn, ok := p.conns[addr]
	p.mu.RUnlock()
	if ok && conn.GetState() != connectivity.Shutdown {
		return conn, nil
	}

	// lock for dialing
	p.mu.Lock()
	defer p.mu.Unlock()

	// re-check after lock for concurrency safeness
	conn, ok = p.conns[addr]
	if ok && conn.GetState() != connectivity.Shutdown {
		return conn, nil
	}

	// ensure connection is closed
	if ok {
		_ = conn.Close()
	}

	newConn, err := grpc.NewClient(addr, p.dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("conn pool: dial %s: %w", addr, err)
	}

	p.conns[addr] = newConn
	return newConn, nil
}

// CloseAll closes every connection in the pool and removes them from the cache.
// Intended to be called once during graceful shutdown.
// All errors are collected and returned as a single joined error.
func (p *ConnectionPool) CloseAll() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errs []error
	for addr, conn := range p.conns {
		if err := conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", addr, err))
		}
		delete(p.conns, addr)
	}

	return errors.Join(errs...)
}
