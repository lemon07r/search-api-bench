// Package testutil provides helpers for provider-level tests.
package testutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

// Server is a lightweight local HTTP test server wrapper.
type Server struct {
	URL       string
	listener  net.Listener
	server    *http.Server
	closeOnce sync.Once
}

// Close shuts down the test server.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		if s.server != nil {
			_ = s.server.Close()
		}
		if s.listener != nil {
			_ = s.listener.Close()
		}
	})
}

// NewIPv4Server creates a local HTTP server bound to 127.0.0.1.
// Tests are skipped when local socket binding is unavailable in the runtime.
func NewIPv4Server(t testing.TB, handler http.Handler) *Server {
	t.Helper()

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping test: unable to bind local tcp4 listener: %v", err)
		return nil
	}

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	testServer := &Server{
		URL:      fmt.Sprintf("http://%s", listener.Addr().String()),
		listener: listener,
		server:   server,
	}

	go func() {
		_ = server.Serve(listener)
	}()

	t.Cleanup(testServer.Close)
	return testServer
}
