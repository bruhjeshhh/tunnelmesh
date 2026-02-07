// Package benchmark provides peer-to-peer speed testing over the mesh network.
package benchmark

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChaosConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ChaosConfig
		wantErr bool
	}{
		{
			name:    "empty config is valid",
			cfg:     ChaosConfig{},
			wantErr: false,
		},
		{
			name: "valid packet loss",
			cfg: ChaosConfig{
				PacketLossPercent: 5.0,
			},
			wantErr: false,
		},
		{
			name: "packet loss at 100%",
			cfg: ChaosConfig{
				PacketLossPercent: 100.0,
			},
			wantErr: false,
		},
		{
			name: "negative packet loss",
			cfg: ChaosConfig{
				PacketLossPercent: -1.0,
			},
			wantErr: true,
		},
		{
			name: "packet loss over 100%",
			cfg: ChaosConfig{
				PacketLossPercent: 101.0,
			},
			wantErr: true,
		},
		{
			name: "valid latency",
			cfg: ChaosConfig{
				Latency: 50 * time.Millisecond,
			},
			wantErr: false,
		},
		{
			name: "negative latency",
			cfg: ChaosConfig{
				Latency: -10 * time.Millisecond,
			},
			wantErr: true,
		},
		{
			name: "valid jitter",
			cfg: ChaosConfig{
				Jitter: 10 * time.Millisecond,
			},
			wantErr: false,
		},
		{
			name: "negative jitter",
			cfg: ChaosConfig{
				Jitter: -5 * time.Millisecond,
			},
			wantErr: true,
		},
		{
			name: "valid bandwidth",
			cfg: ChaosConfig{
				BandwidthBps: 1000000, // 1 MB/s
			},
			wantErr: false,
		},
		{
			name: "negative bandwidth",
			cfg: ChaosConfig{
				BandwidthBps: -1000,
			},
			wantErr: true,
		},
		{
			name: "all options combined",
			cfg: ChaosConfig{
				PacketLossPercent: 5.0,
				Latency:           50 * time.Millisecond,
				Jitter:            10 * time.Millisecond,
				BandwidthBps:      10000000,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestChaosConfig_IsEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  ChaosConfig
		want bool
	}{
		{
			name: "empty config",
			cfg:  ChaosConfig{},
			want: false,
		},
		{
			name: "packet loss enabled",
			cfg:  ChaosConfig{PacketLossPercent: 5.0},
			want: true,
		},
		{
			name: "latency enabled",
			cfg:  ChaosConfig{Latency: 50 * time.Millisecond},
			want: true,
		},
		{
			name: "jitter enabled",
			cfg:  ChaosConfig{Jitter: 10 * time.Millisecond},
			want: true,
		},
		{
			name: "bandwidth enabled",
			cfg:  ChaosConfig{BandwidthBps: 1000000},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.IsEnabled()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid minimal config",
			cfg: Config{
				PeerName: "peer-1",
				Size:     1024,
			},
			wantErr: false,
		},
		{
			name: "missing peer name",
			cfg: Config{
				Size: 1024,
			},
			wantErr: true,
		},
		{
			name: "zero size",
			cfg: Config{
				PeerName: "peer-1",
				Size:     0,
			},
			wantErr: true,
		},
		{
			name: "negative size",
			cfg: Config{
				PeerName: "peer-1",
				Size:     -100,
			},
			wantErr: true,
		},
		{
			name: "valid upload direction",
			cfg: Config{
				PeerName:  "peer-1",
				Size:      1024,
				Direction: DirectionUpload,
			},
			wantErr: false,
		},
		{
			name: "valid download direction",
			cfg: Config{
				PeerName:  "peer-1",
				Size:      1024,
				Direction: DirectionDownload,
			},
			wantErr: false,
		},
		{
			name: "invalid direction",
			cfg: Config{
				PeerName:  "peer-1",
				Size:      1024,
				Direction: "sideways",
			},
			wantErr: true,
		},
		{
			name: "with valid chaos config",
			cfg: Config{
				PeerName: "peer-1",
				Size:     1024,
				Chaos: ChaosConfig{
					PacketLossPercent: 5.0,
				},
			},
			wantErr: false,
		},
		{
			name: "with invalid chaos config",
			cfg: Config{
				PeerName: "peer-1",
				Size:     1024,
				Chaos: ChaosConfig{
					PacketLossPercent: 150.0,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestResult_JSON(t *testing.T) {
	original := &Result{
		ID:              "bench-123",
		Timestamp:       time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		LocalPeer:       "peer-1",
		RemotePeer:      "peer-2",
		Direction:       DirectionUpload,
		RequestedSize:   100 * 1024 * 1024,
		TransferredSize: 100 * 1024 * 1024,
		DurationMs:      5000,
		ThroughputBps:   20 * 1024 * 1024,
		ThroughputMbps:  160.0,
		LatencyMinMs:    1.5,
		LatencyMaxMs:    15.2,
		LatencyAvgMs:    5.3,
		Success:         true,
		Chaos: &ChaosConfig{
			PacketLossPercent: 5.0,
			Latency:           50 * time.Millisecond,
		},
	}

	// Marshal
	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal
	var decoded Result
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify key fields
	assert.Equal(t, original.ID, decoded.ID)
	assert.Equal(t, original.LocalPeer, decoded.LocalPeer)
	assert.Equal(t, original.RemotePeer, decoded.RemotePeer)
	assert.Equal(t, original.Direction, decoded.Direction)
	assert.Equal(t, original.RequestedSize, decoded.RequestedSize)
	assert.Equal(t, original.ThroughputBps, decoded.ThroughputBps)
	assert.Equal(t, original.Success, decoded.Success)
	assert.NotNil(t, decoded.Chaos)
	assert.Equal(t, original.Chaos.PacketLossPercent, decoded.Chaos.PacketLossPercent)
}

func TestResult_Error(t *testing.T) {
	result := &Result{
		ID:         "bench-456",
		LocalPeer:  "peer-1",
		RemotePeer: "peer-2",
		Success:    false,
		Error:      "connection refused",
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var decoded Result
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.False(t, decoded.Success)
	assert.Equal(t, "connection refused", decoded.Error)
}

func TestConfig_WithDefaults(t *testing.T) {
	cfg := Config{
		PeerName: "peer-1",
		Size:     1024,
	}

	cfg = cfg.WithDefaults()

	assert.Equal(t, DirectionUpload, cfg.Direction)
	assert.Equal(t, DefaultPort, cfg.Port)
	assert.Equal(t, DefaultTimeout, cfg.Timeout)
}
