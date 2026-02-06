package admin

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"

	"github.com/tunnelmesh/tunnelmesh/internal/metrics"
)

// AdminServer provides an HTTPS admin interface for peer metrics and health checks.
// It serves on the mesh IP only (not exposed to the public internet).
type AdminServer struct {
	server *http.Server
	mux    *http.ServeMux
}

// NewAdminServer creates a new peer admin server.
func NewAdminServer() *AdminServer {
	mux := http.NewServeMux()

	// Register handlers
	mux.HandleFunc("/health", healthHandler)
	mux.Handle("/metrics", metrics.Handler())

	return &AdminServer{
		mux: mux,
	}
}

// Start starts the admin server with TLS on the given address.
func (s *AdminServer) Start(addr string, cert *tls.Certificate) error {
	s.server = &http.Server{
		Addr:    addr,
		Handler: s.mux,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{*cert},
			MinVersion:   tls.VersionTLS12,
		},
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		// Use ListenAndServeTLS with empty strings since certs are in TLSConfig
		if err := s.server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			// Log error but don't crash - metrics are optional
		}
	}()

	return nil
}

// StartInsecure starts the admin server without TLS (for testing only).
func (s *AdminServer) StartInsecure(addr string) error {
	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log error but don't crash - metrics are optional
		}
	}()

	return nil
}

// Stop gracefully stops the admin server.
func (s *AdminServer) Stop() error {
	if s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

// healthHandler returns a simple health check response.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok\n"))
}
