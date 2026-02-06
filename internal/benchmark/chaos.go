package benchmark

import (
	"io"
	"math/rand"
	"sync"
	"time"
)

// TokenBucket implements a token bucket rate limiter.
type TokenBucket struct {
	mu         sync.Mutex
	rate       float64   // bytes per second
	tokens     float64   // current tokens
	maxTokens  float64   // bucket size (1 second worth)
	lastUpdate time.Time // last token update time
}

// NewTokenBucket creates a new token bucket with the given rate in bytes per second.
func NewTokenBucket(bytesPerSec int64) *TokenBucket {
	return &TokenBucket{
		rate:       float64(bytesPerSec),
		tokens:     float64(bytesPerSec), // Start with full bucket
		maxTokens:  float64(bytesPerSec), // 1 second worth of tokens
		lastUpdate: time.Now(),
	}
}

// Take attempts to take n tokens from the bucket.
// Returns the duration to wait before the tokens are available.
// If the returned duration is 0, the tokens were immediately available.
func (tb *TokenBucket) Take(n int) time.Duration {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastUpdate).Seconds()
	tb.lastUpdate = now

	// Refill tokens based on elapsed time
	tb.tokens += elapsed * tb.rate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}

	// Check if we have enough tokens
	needed := float64(n)
	if tb.tokens >= needed {
		tb.tokens -= needed
		return 0
	}

	// Calculate wait time for remaining tokens
	deficit := needed - tb.tokens
	waitTime := time.Duration(deficit / tb.rate * float64(time.Second))

	// Take what we have and go into deficit
	tb.tokens = 0

	return waitTime
}

// ChaosWriterStats contains statistics about the ChaosWriter's behavior.
type ChaosWriterStats struct {
	TotalWrites   int64 // Total number of Write() calls
	DroppedWrites int64 // Number of writes dropped due to packet loss
	TotalBytes    int64 // Total bytes passed to Write()
	ActualBytes   int64 // Actual bytes written to underlying writer
}

// ChaosWriter wraps an io.Writer and applies chaos testing effects.
type ChaosWriter struct {
	w      io.Writer
	cfg    ChaosConfig
	rng    *rand.Rand
	bucket *TokenBucket

	mu    sync.Mutex
	stats ChaosWriterStats
}

// NewChaosWriter creates a new ChaosWriter with the given configuration.
func NewChaosWriter(w io.Writer, cfg ChaosConfig) *ChaosWriter {
	return NewChaosWriterWithRng(w, cfg, rand.New(rand.NewSource(time.Now().UnixNano())))
}

// NewChaosWriterWithRng creates a new ChaosWriter with a specific random source.
// This is useful for testing with reproducible randomness.
func NewChaosWriterWithRng(w io.Writer, cfg ChaosConfig, rng *rand.Rand) *ChaosWriter {
	cw := &ChaosWriter{
		w:   w,
		cfg: cfg,
		rng: rng,
	}

	if cfg.BandwidthBps > 0 {
		cw.bucket = NewTokenBucket(cfg.BandwidthBps)
	}

	return cw
}

// Write implements io.Writer with chaos effects applied.
func (c *ChaosWriter) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.stats.TotalWrites++
	c.stats.TotalBytes += int64(len(p))
	c.mu.Unlock()

	// 1. Check packet loss - randomly drop
	if c.cfg.PacketLossPercent > 0 {
		if c.rng.Float64()*100 < c.cfg.PacketLossPercent {
			c.mu.Lock()
			c.stats.DroppedWrites++
			c.mu.Unlock()
			// Pretend we wrote successfully
			return len(p), nil
		}
	}

	// 2. Apply bandwidth limit - wait for tokens
	if c.bucket != nil {
		wait := c.bucket.Take(len(p))
		if wait > 0 {
			time.Sleep(wait)
		}
	}

	// 3. Add latency + jitter
	if c.cfg.Latency > 0 || c.cfg.Jitter > 0 {
		delay := c.cfg.Latency

		if c.cfg.Jitter > 0 {
			// Add random jitter between -jitter and +jitter
			jitterRange := 2 * int64(c.cfg.Jitter)
			jitterOffset := time.Duration(c.rng.Int63n(jitterRange)) - c.cfg.Jitter
			delay += jitterOffset

			// Ensure delay doesn't go negative
			if delay < 0 {
				delay = 0
			}
		}

		if delay > 0 {
			time.Sleep(delay)
		}
	}

	// 4. Write to underlying writer
	n, err := c.w.Write(p)

	c.mu.Lock()
	c.stats.ActualBytes += int64(n)
	c.mu.Unlock()

	return n, err
}

// Stats returns the current statistics for this ChaosWriter.
func (c *ChaosWriter) Stats() ChaosWriterStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

// Reset resets the statistics.
func (c *ChaosWriter) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats = ChaosWriterStats{}
}
