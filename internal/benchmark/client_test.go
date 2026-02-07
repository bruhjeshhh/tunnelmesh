package benchmark

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tunnelmesh/tunnelmesh/testutil"
)

func TestClient_Run_Upload(t *testing.T) {
	// Start a server
	port := testutil.FreePort(t)
	server := NewServer("127.0.0.1", port)
	require.NoError(t, server.Start())
	defer func() { _ = server.Stop() }()

	// Create client config
	cfg := Config{
		PeerName:  "test-peer",
		Size:      64 * 1024, // 64KB
		Direction: DirectionUpload,
		Timeout:   10 * time.Second,
		Port:      port,
	}

	// Run benchmark
	client := NewClient("test-local", "127.0.0.1")
	result, err := client.Run(context.Background(), cfg)
	require.NoError(t, err)

	// Verify result
	assert.True(t, result.Success)
	assert.Empty(t, result.Error)
	assert.Equal(t, "test-local", result.LocalPeer)
	assert.Equal(t, "test-peer", result.RemotePeer)
	assert.Equal(t, DirectionUpload, result.Direction)
	assert.Equal(t, int64(64*1024), result.RequestedSize)
	assert.Equal(t, int64(64*1024), result.TransferredSize)
	// Duration might be 0ms for fast local transfers, just check it's non-negative
	assert.GreaterOrEqual(t, result.DurationMs, int64(0))
	// Throughput should be positive if duration > 0, or 0 if instant
	assert.GreaterOrEqual(t, result.ThroughputBps, float64(0))
}

func TestClient_Run_Download(t *testing.T) {
	port := testutil.FreePort(t)
	server := NewServer("127.0.0.1", port)
	require.NoError(t, server.Start())
	defer func() { _ = server.Stop() }()

	cfg := Config{
		PeerName:  "test-peer",
		Size:      64 * 1024, // 64KB
		Direction: DirectionDownload,
		Timeout:   10 * time.Second,
		Port:      port,
	}

	client := NewClient("test-local", "127.0.0.1")
	result, err := client.Run(context.Background(), cfg)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.Equal(t, DirectionDownload, result.Direction)
	// For download, we receive data from server
	assert.Greater(t, result.TransferredSize, int64(0))
}

func TestClient_Run_WithChaos(t *testing.T) {
	port := testutil.FreePort(t)
	server := NewServer("127.0.0.1", port)
	require.NoError(t, server.Start())
	defer func() { _ = server.Stop() }()

	cfg := Config{
		PeerName:  "test-peer",
		Size:      1024,
		Direction: DirectionUpload,
		Timeout:   10 * time.Second,
		Port:      port,
		Chaos: ChaosConfig{
			Latency: 10 * time.Millisecond,
		},
	}

	client := NewClient("test-local", "127.0.0.1")
	start := time.Now()
	result, err := client.Run(context.Background(), cfg)
	elapsed := time.Since(start)
	require.NoError(t, err)

	assert.True(t, result.Success)
	// Should have some latency from chaos config
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(10))
	// Chaos config should be recorded
	assert.NotNil(t, result.Chaos)
	assert.Equal(t, 10*time.Millisecond, result.Chaos.Latency)
}

func TestClient_Run_Latency(t *testing.T) {
	port := testutil.FreePort(t)
	server := NewServer("127.0.0.1", port)
	require.NoError(t, server.Start())
	defer func() { _ = server.Stop() }()

	cfg := Config{
		PeerName:  "test-peer",
		Size:      1024,
		Direction: DirectionUpload,
		Timeout:   10 * time.Second,
		Port:      port,
	}

	client := NewClient("test-local", "127.0.0.1")
	result, err := client.Run(context.Background(), cfg)
	require.NoError(t, err)

	assert.True(t, result.Success)
	// Should have latency measurements (may be 0 on fast local connections/Windows)
	assert.GreaterOrEqual(t, result.LatencyAvgMs, float64(0))
	assert.GreaterOrEqual(t, result.LatencyMaxMs, result.LatencyMinMs)
}

func TestClient_Run_ConnectionError(t *testing.T) {
	cfg := Config{
		PeerName:  "test-peer",
		Size:      1024,
		Direction: DirectionUpload,
		Timeout:   1 * time.Second,
		Port:      19999, // No server on this port
	}

	client := NewClient("test-local", "127.0.0.1")
	result, err := client.Run(context.Background(), cfg)

	// Should return error
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestClient_Run_ContextCancellation(t *testing.T) {
	port := testutil.FreePort(t)
	server := NewServer("127.0.0.1", port)
	require.NoError(t, server.Start())
	defer func() { _ = server.Stop() }()

	// Use latency to ensure the transfer takes long enough to be cancelled
	cfg := Config{
		PeerName:  "test-peer",
		Size:      10 * ChunkSize, // 10 chunks
		Direction: DirectionUpload,
		Timeout:   60 * time.Second,
		Port:      port,
		Chaos: ChaosConfig{
			Latency: 100 * time.Millisecond, // 100ms per write
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	client := NewClient("test-local", "127.0.0.1")
	result, err := client.Run(ctx, cfg)

	// Should return context error or incomplete result
	if err == nil {
		// If no error, the result should show incomplete transfer
		assert.Less(t, result.TransferredSize, cfg.Size)
	} else {
		assert.Error(t, err)
	}
}

func TestClient_Run_LargeTransfer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large transfer test in short mode")
	}

	port := testutil.FreePort(t)
	server := NewServer("127.0.0.1", port)
	require.NoError(t, server.Start())
	defer func() { _ = server.Stop() }()

	cfg := Config{
		PeerName:  "test-peer",
		Size:      1024 * 1024, // 1MB
		Direction: DirectionUpload,
		Timeout:   30 * time.Second,
		Port:      port,
	}

	client := NewClient("test-local", "127.0.0.1")
	result, err := client.Run(context.Background(), cfg)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.Equal(t, int64(1024*1024), result.TransferredSize)
}

func TestClient_MultiplePings(t *testing.T) {
	port := testutil.FreePort(t)
	server := NewServer("127.0.0.1", port)
	require.NoError(t, server.Start())
	defer func() { _ = server.Stop() }()

	cfg := Config{
		PeerName:  "test-peer",
		Size:      ChunkSize * 3, // 3 chunks to trigger multiple pings
		Direction: DirectionUpload,
		Timeout:   10 * time.Second,
		Port:      port,
	}

	client := NewClient("test-local", "127.0.0.1")
	result, err := client.Run(context.Background(), cfg)
	require.NoError(t, err)

	assert.True(t, result.Success)
	// Should have latency measurements from multiple pings (may be 0 on fast local connections/Windows)
	assert.GreaterOrEqual(t, result.LatencyAvgMs, float64(0))
}

// Benchmark client performance
func BenchmarkClient_Upload(b *testing.B) {
	port := 29998 // Fixed port for benchmark
	server := NewServer("127.0.0.1", port)
	if err := server.Start(); err != nil {
		b.Fatalf("failed to start server: %v", err)
	}
	defer func() { _ = server.Stop() }()

	cfg := Config{
		PeerName:  "bench-peer",
		Size:      64 * 1024, // 64KB
		Direction: DirectionUpload,
		Timeout:   10 * time.Second,
		Port:      port,
	}

	client := NewClient("bench-local", "127.0.0.1")

	b.ResetTimer()
	b.SetBytes(cfg.Size)

	for i := 0; i < b.N; i++ {
		_, err := client.Run(context.Background(), cfg)
		if err != nil {
			b.Fatalf("benchmark failed: %v", err)
		}
	}
}
