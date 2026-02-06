// SD Generator for Prometheus file_sd
// Polls the TunnelMesh coordination server and generates targets file for Prometheus.
package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Target represents a Prometheus file_sd target entry.
type Target struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

// Peer represents a peer from the coordination server API.
type Peer struct {
	Name   string `json:"name"`
	MeshIP string `json:"mesh_ip"`
	Online bool   `json:"online"`
}

// PeersResponse represents the API response from /api/v1/peers.
type PeersResponse struct {
	Peers []Peer `json:"peers"`
}

func main() {
	coordURL := getEnv("COORD_SERVER_URL", "https://localhost:443")
	authToken := getEnv("AUTH_TOKEN", "")
	pollInterval := getEnvDuration("POLL_INTERVAL", 30*time.Second)
	outputFile := getEnv("OUTPUT_FILE", "/targets/peers.json")
	metricsPort := getEnv("METRICS_PORT", "9443")
	tlsSkipVerify := getEnv("TLS_SKIP_VERIFY", "false") == "true"

	fmt.Printf("Starting SD generator\n")
	fmt.Printf("  Coord server: %s\n", coordURL)
	fmt.Printf("  Poll interval: %s\n", pollInterval)
	fmt.Printf("  Output file: %s\n", outputFile)
	fmt.Printf("  Metrics port: %s\n", metricsPort)
	fmt.Printf("  TLS skip verify: %t\n", tlsSkipVerify)

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: tlsSkipVerify,
			},
		},
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Run immediately on start
	if err := generateTargets(client, coordURL, authToken, outputFile, metricsPort); err != nil {
		fmt.Printf("Error generating targets: %v\n", err)
	}

	for range ticker.C {
		if err := generateTargets(client, coordURL, authToken, outputFile, metricsPort); err != nil {
			fmt.Printf("Error generating targets: %v\n", err)
		}
	}
}

func generateTargets(client *http.Client, coordURL, authToken, outputFile, metricsPort string) error {
	url := fmt.Sprintf("%s/api/v1/peers", coordURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch peers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var peersResp PeersResponse
	if err := json.NewDecoder(resp.Body).Decode(&peersResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	var targets []Target
	for _, peer := range peersResp.Peers {
		if !peer.Online || peer.MeshIP == "" {
			continue
		}
		targets = append(targets, Target{
			Targets: []string{fmt.Sprintf("%s:%s", peer.MeshIP, metricsPort)},
			Labels: map[string]string{
				"peer": peer.Name,
			},
		})
	}

	data, err := json.MarshalIndent(targets, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal targets: %w", err)
	}

	// Write atomically using temp file
	tmpFile := outputFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tmpFile, outputFile); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	fmt.Printf("Generated %d targets\n", len(targets))
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
