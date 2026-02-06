// Benchmarker service for Docker - runs periodic benchmarks between mesh peers.
// Results are written to JSON files for analysis.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tunnelmesh/tunnelmesh/internal/benchmark"
	"github.com/tunnelmesh/tunnelmesh/pkg/bytesize"
)

type Config struct {
	// CoordURL is the coordination server URL for fetching peer list
	CoordURL string

	// AuthToken for authentication with the coord server
	AuthToken string

	// LocalPeer is the name of this peer (for benchmark results)
	LocalPeer string

	// Interval between benchmark runs
	Interval time.Duration

	// Size of data to transfer in each benchmark
	Size int64

	// OutputDir for JSON result files
	OutputDir string

	// TLSSkipVerify disables TLS certificate verification
	TLSSkipVerify bool
}

func main() {
	cfg := configFromEnv()

	fmt.Printf("Starting benchmarker\n")
	fmt.Printf("  Coord server: %s\n", cfg.CoordURL)
	fmt.Printf("  Local peer:   %s\n", cfg.LocalPeer)
	fmt.Printf("  Interval:     %s\n", cfg.Interval)
	fmt.Printf("  Size:         %s\n", bytesize.Format(cfg.Size))
	fmt.Printf("  Output dir:   %s\n", cfg.OutputDir)

	// Ensure output directory exists
	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	// Wait a bit for mesh to stabilize before first run
	time.Sleep(30 * time.Second)

	// Run immediately on start
	runBenchmarks(cfg)

	for range ticker.C {
		runBenchmarks(cfg)
	}
}

func runBenchmarks(cfg Config) {
	peers, err := fetchPeers(cfg)
	if err != nil {
		fmt.Printf("Error fetching peers: %v\n", err)
		return
	}

	if len(peers) == 0 {
		fmt.Println("No peers available for benchmark")
		return
	}

	// Select up to 2 random peers to benchmark
	selectedPeers := selectRandomPeers(peers, 2)

	fmt.Printf("Running benchmarks against %d peers\n", len(selectedPeers))

	for _, peer := range selectedPeers {
		result := runBenchmark(cfg, peer)
		if result != nil {
			saveResult(cfg, result)
		}
	}
}

type peerInfo struct {
	Name   string `json:"name"`
	MeshIP string `json:"mesh_ip"`
}

func fetchPeers(cfg Config) ([]peerInfo, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.TLSSkipVerify},
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", cfg.CoordURL+"/api/v1/peers", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var peers []peerInfo
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return nil, err
	}

	// Filter out self
	var filtered []peerInfo
	for _, p := range peers {
		if p.Name != cfg.LocalPeer {
			filtered = append(filtered, p)
		}
	}

	return filtered, nil
}

func selectRandomPeers(peers []peerInfo, n int) []peerInfo {
	if len(peers) <= n {
		return peers
	}

	// Shuffle and take first n
	shuffled := make([]peerInfo, len(peers))
	copy(shuffled, peers)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	return shuffled[:n]
}

func runBenchmark(cfg Config, peer peerInfo) *benchmark.Result {
	fmt.Printf("  Benchmarking %s (%s)...\n", peer.Name, peer.MeshIP)

	benchCfg := benchmark.Config{
		PeerName:  peer.Name,
		Size:      cfg.Size,
		Direction: benchmark.DirectionUpload,
		Timeout:   120 * time.Second,
		Port:      benchmark.DefaultPort,
	}

	client := benchmark.NewClient(cfg.LocalPeer, peer.MeshIP)
	ctx, cancel := context.WithTimeout(context.Background(), benchCfg.Timeout)
	defer cancel()

	result, err := client.Run(ctx, benchCfg)
	if err != nil {
		fmt.Printf("    Error: %v\n", err)
		return nil
	}

	fmt.Printf("    Throughput: %s\n", bytesize.FormatRate(int64(result.ThroughputBps)))
	fmt.Printf("    Latency:    %.2f ms (avg)\n", result.LatencyAvgMs)

	return result
}

func saveResult(cfg Config, result *benchmark.Result) {
	// Create filename with timestamp
	filename := fmt.Sprintf("benchmark_%s_%s_%s.json",
		result.LocalPeer,
		result.RemotePeer,
		result.Timestamp.Format("20060102_150405"))
	path := filepath.Join(cfg.OutputDir, filename)

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Printf("    Error marshaling result: %v\n", err)
		return
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Printf("    Error writing result: %v\n", err)
		return
	}

	fmt.Printf("    Saved: %s\n", filename)
}

func configFromEnv() Config {
	cfg := Config{
		CoordURL:  "http://localhost:8080",
		LocalPeer: "benchmarker",
		Interval:  5 * time.Minute,
		Size:      100 * 1024 * 1024, // 100MB
		OutputDir: "/results",
	}

	if v := os.Getenv("COORD_SERVER_URL"); v != "" {
		cfg.CoordURL = v
	}
	if v := os.Getenv("AUTH_TOKEN"); v != "" {
		cfg.AuthToken = v
	}
	if v := os.Getenv("LOCAL_PEER"); v != "" {
		cfg.LocalPeer = v
	}
	if v := os.Getenv("BENCHMARK_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Interval = d
		}
	}
	if v := os.Getenv("BENCHMARK_SIZE"); v != "" {
		if size, err := bytesize.Parse(v); err == nil {
			cfg.Size = size
		}
	}
	if v := os.Getenv("OUTPUT_DIR"); v != "" {
		cfg.OutputDir = v
	}
	if os.Getenv("TLS_SKIP_VERIFY") == "true" {
		cfg.TLSSkipVerify = true
	}

	return cfg
}
