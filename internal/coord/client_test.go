package coord

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tunnelmesh/tunnelmesh/internal/config"
)

func TestClient_Register(t *testing.T) {
	// Create test server
	cfg := &config.ServerConfig{
		Listen:       ":0",
		AuthToken:    "test-token",
		MeshCIDR:     "10.99.0.0/16",
		DomainSuffix: ".tunnelmesh",
	}
	srv, err := NewServer(cfg)
	require.NoError(t, err)

	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Create client
	client := NewClient(ts.URL, "test-token")

	// Register
	resp, err := client.Register("mynode", "SHA256:abc123", []string{"1.2.3.4"}, []string{"192.168.1.1"}, 2222, 0, false, "v1.0.0")
	require.NoError(t, err)

	assert.Contains(t, resp.MeshIP, "10.99.") // IP is hash-based, just check it's in mesh range
	assert.Equal(t, "10.99.0.0/16", resp.MeshCIDR)
	assert.Equal(t, ".tunnelmesh", resp.Domain)
}

func TestClient_ListPeers(t *testing.T) {
	cfg := &config.ServerConfig{
		Listen:       ":0",
		AuthToken:    "test-token",
		MeshCIDR:     "10.99.0.0/16",
		DomainSuffix: ".tunnelmesh",
	}
	srv, err := NewServer(cfg)
	require.NoError(t, err)

	ts := httptest.NewServer(srv)
	defer ts.Close()

	client := NewClient(ts.URL, "test-token")

	// Register a peer
	_, err = client.Register("node1", "SHA256:key1", nil, nil, 2222, 0, false, "v1.0.0")
	require.NoError(t, err)

	// List peers
	peers, err := client.ListPeers()
	require.NoError(t, err)

	assert.Len(t, peers, 1)
	assert.Equal(t, "node1", peers[0].Name)
}

// Note: TestClient_Heartbeat and TestClient_HeartbeatNotFound removed.
// Heartbeats are now sent via WebSocket using PersistentRelay.SendHeartbeat().
// See internal/tunnel/persistent_relay_test.go for WebSocket heartbeat tests.

func TestClient_Deregister(t *testing.T) {
	cfg := &config.ServerConfig{
		Listen:       ":0",
		AuthToken:    "test-token",
		MeshCIDR:     "10.99.0.0/16",
		DomainSuffix: ".tunnelmesh",
	}
	srv, err := NewServer(cfg)
	require.NoError(t, err)

	ts := httptest.NewServer(srv)
	defer ts.Close()

	client := NewClient(ts.URL, "test-token")

	// Register first
	_, err = client.Register("mynode", "SHA256:key", nil, nil, 2222, 0, false, "v1.0.0")
	require.NoError(t, err)

	// Verify registered
	peers, _ := client.ListPeers()
	assert.Len(t, peers, 1)

	// Deregister
	err = client.Deregister("mynode")
	assert.NoError(t, err)

	// Verify gone
	peers, _ = client.ListPeers()
	assert.Len(t, peers, 0)
}

func TestClient_GetDNSRecords(t *testing.T) {
	cfg := &config.ServerConfig{
		Listen:       ":0",
		AuthToken:    "test-token",
		MeshCIDR:     "10.99.0.0/16",
		DomainSuffix: ".tunnelmesh",
	}
	srv, err := NewServer(cfg)
	require.NoError(t, err)

	ts := httptest.NewServer(srv)
	defer ts.Close()

	client := NewClient(ts.URL, "test-token")

	// Register peers
	_, err = client.Register("node1", "SHA256:key1", nil, nil, 2222, 0, false, "v1.0.0")
	require.NoError(t, err)
	_, err = client.Register("node2", "SHA256:key2", nil, nil, 2222, 0, false, "v1.0.0")
	require.NoError(t, err)

	// Get DNS records
	records, err := client.GetDNSRecords()
	require.NoError(t, err)

	assert.Len(t, records, 2)
}

func TestClient_RegisterWithRetry_SuccessOnFirstTry(t *testing.T) {
	cfg := &config.ServerConfig{
		Listen:       ":0",
		AuthToken:    "test-token",
		MeshCIDR:     "10.99.0.0/16",
		DomainSuffix: ".tunnelmesh",
	}
	srv, err := NewServer(cfg)
	require.NoError(t, err)

	ts := httptest.NewServer(srv)
	defer ts.Close()

	client := NewClient(ts.URL, "test-token")

	ctx := context.Background()
	retryCfg := RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
	}

	resp, err := client.RegisterWithRetry(ctx, "mynode", "SHA256:abc123", []string{"1.2.3.4"}, nil, 2222, 2223, false, "v1.0.0", retryCfg)
	require.NoError(t, err)

	assert.Contains(t, resp.MeshIP, "10.99.")
	assert.Equal(t, "10.99.0.0/16", resp.MeshCIDR)
}

func TestClient_RegisterWithRetry_SuccessAfterFailures(t *testing.T) {
	var attempts atomic.Int32

	// Create a server that fails the first 2 attempts then succeeds
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attempts.Add(1)
		if attempt <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error": "unavailable", "message": "server starting"}`))
			return
		}

		// Success on 3rd attempt
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"mesh_ip": "10.99.1.1",
			"mesh_cidr": "10.99.0.0/16",
			"domain": ".tunnelmesh",
			"token": "jwt-token"
		}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "test-token")

	ctx := context.Background()
	retryCfg := RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
	}

	resp, err := client.RegisterWithRetry(ctx, "mynode", "SHA256:abc123", nil, nil, 2222, 2223, false, "v1.0.0", retryCfg)
	require.NoError(t, err)

	assert.Equal(t, int32(3), attempts.Load(), "should have taken 3 attempts")
	assert.Equal(t, "10.99.1.1", resp.MeshIP)
}

func TestClient_RegisterWithRetry_MaxRetriesExceeded(t *testing.T) {
	var attempts atomic.Int32

	// Create a server that always fails
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error": "unavailable", "message": "server down"}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "test-token")

	ctx := context.Background()
	retryCfg := RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
	}

	_, err := client.RegisterWithRetry(ctx, "mynode", "SHA256:abc123", nil, nil, 2222, 2223, false, "v1.0.0", retryCfg)
	require.Error(t, err)

	assert.Equal(t, int32(3), attempts.Load(), "should have made exactly 3 attempts")
	assert.Contains(t, err.Error(), "after 3 attempts")
}

func TestClient_RegisterWithRetry_ContextCancelled(t *testing.T) {
	var attempts atomic.Int32

	// Create a server that always fails
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error": "unavailable"}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "test-token")

	ctx, cancel := context.WithCancel(context.Background())
	retryCfg := RetryConfig{
		MaxRetries:     10,
		InitialBackoff: 100 * time.Millisecond, // Long enough to cancel during wait
		MaxBackoff:     1 * time.Second,
	}

	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := client.RegisterWithRetry(ctx, "mynode", "SHA256:abc123", nil, nil, 2222, 2223, false, "v1.0.0", retryCfg)
	require.Error(t, err)

	assert.Equal(t, context.Canceled, err)
	assert.LessOrEqual(t, attempts.Load(), int32(2), "should stop early due to cancellation")
}

func TestClient_RegisterWithRetry_ConnectionRefused(t *testing.T) {
	// Client pointing to a closed server (connection refused)
	client := NewClient("http://127.0.0.1:59999", "test-token")

	ctx := context.Background()
	retryCfg := RetryConfig{
		MaxRetries:     2,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
	}

	_, err := client.RegisterWithRetry(ctx, "mynode", "SHA256:abc123", nil, nil, 2222, 2223, false, "v1.0.0", retryCfg)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "after 2 attempts")
}
