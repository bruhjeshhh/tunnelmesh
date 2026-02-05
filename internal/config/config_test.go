package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tunnelmesh/tunnelmesh/testutil"
)

func TestLoadServerConfig(t *testing.T) {
	dir, cleanup := testutil.TempDir(t)
	defer cleanup()

	content := `
listen: ":8080"
auth_token: "test-token-123"
mesh_cidr: "10.99.0.0/16"
domain_suffix: ".tunnelmesh"
`
	configPath := testutil.TempFile(t, dir, "server.yaml", content)

	cfg, err := LoadServerConfig(configPath)
	require.NoError(t, err)

	assert.Equal(t, ":8080", cfg.Listen)
	assert.Equal(t, "test-token-123", cfg.AuthToken)
	assert.Equal(t, "10.99.0.0/16", cfg.MeshCIDR)
	assert.Equal(t, ".tunnelmesh", cfg.DomainSuffix)
}

func TestLoadServerConfig_Defaults(t *testing.T) {
	dir, cleanup := testutil.TempDir(t)
	defer cleanup()

	// Minimal config with only required fields
	content := `
listen: ":9000"
auth_token: "secret"
`
	configPath := testutil.TempFile(t, dir, "server.yaml", content)

	cfg, err := LoadServerConfig(configPath)
	require.NoError(t, err)

	assert.Equal(t, ":9000", cfg.Listen)
	assert.Equal(t, "secret", cfg.AuthToken)
	// Check defaults
	assert.Equal(t, "10.99.0.0/16", cfg.MeshCIDR)
	assert.Equal(t, ".tunnelmesh", cfg.DomainSuffix)
}

func TestLoadServerConfig_FileNotFound(t *testing.T) {
	_, err := LoadServerConfig("/nonexistent/path/config.yaml")
	assert.Error(t, err)
}

func TestLoadServerConfig_InvalidYAML(t *testing.T) {
	dir, cleanup := testutil.TempDir(t)
	defer cleanup()

	content := `
listen: [invalid yaml
`
	configPath := testutil.TempFile(t, dir, "server.yaml", content)

	_, err := LoadServerConfig(configPath)
	assert.Error(t, err)
}

func TestLoadPeerConfig(t *testing.T) {
	dir, cleanup := testutil.TempDir(t)
	defer cleanup()

	content := `
name: "mynode"
server: "https://coord.example.com"
auth_token: "peer-token"
ssh_port: 2222
private_key: "/path/to/key"
tun:
  name: "tun-mesh0"
  mtu: 1400
dns:
  enabled: true
  listen: "127.0.0.53:5353"
  cache_ttl: 60
`
	configPath := testutil.TempFile(t, dir, "peer.yaml", content)

	cfg, err := LoadPeerConfig(configPath)
	require.NoError(t, err)

	assert.Equal(t, "mynode", cfg.Name)
	assert.Equal(t, "https://coord.example.com", cfg.Server)
	assert.Equal(t, "peer-token", cfg.AuthToken)
	assert.Equal(t, 2222, cfg.SSHPort)
	assert.Equal(t, "/path/to/key", cfg.PrivateKey)
	assert.Equal(t, "tun-mesh0", cfg.TUN.Name)
	assert.Equal(t, 1400, cfg.TUN.MTU)
	assert.True(t, cfg.DNS.Enabled)
	assert.Equal(t, "127.0.0.53:5353", cfg.DNS.Listen)
	assert.Equal(t, 60, cfg.DNS.CacheTTL)
}

func TestLoadPeerConfig_Defaults(t *testing.T) {
	dir, cleanup := testutil.TempDir(t)
	defer cleanup()

	content := `
name: "testnode"
server: "http://localhost:8080"
auth_token: "token"
`
	configPath := testutil.TempFile(t, dir, "peer.yaml", content)

	cfg, err := LoadPeerConfig(configPath)
	require.NoError(t, err)

	// Check defaults
	assert.Equal(t, 2222, cfg.SSHPort)
	assert.Equal(t, "tun-mesh0", cfg.TUN.Name)
	assert.Equal(t, 1400, cfg.TUN.MTU)
	assert.True(t, cfg.DNS.Enabled)
	assert.Equal(t, "127.0.0.53:5353", cfg.DNS.Listen)
	assert.Equal(t, 300, cfg.DNS.CacheTTL)
}

func TestLoadPeerConfig_ExpandHomePath(t *testing.T) {
	dir, cleanup := testutil.TempDir(t)
	defer cleanup()

	content := `
name: "testnode"
server: "http://localhost:8080"
auth_token: "token"
private_key: "~/.tunnelmesh/id_ed25519"
`
	configPath := testutil.TempFile(t, dir, "peer.yaml", content)

	cfg, err := LoadPeerConfig(configPath)
	require.NoError(t, err)

	// Should expand ~ to home directory
	homeDir, _ := os.UserHomeDir()
	expected := filepath.Join(homeDir, ".tunnelmesh/id_ed25519")
	assert.Equal(t, expected, cfg.PrivateKey)
}

func TestServerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ServerConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: ServerConfig{
				Listen:       ":8080",
				AuthToken:    "token",
				MeshCIDR:     "10.99.0.0/16",
				DomainSuffix: ".tunnelmesh",
			},
			wantErr: false,
		},
		{
			name: "missing listen",
			cfg: ServerConfig{
				AuthToken:    "token",
				MeshCIDR:     "10.99.0.0/16",
				DomainSuffix: ".tunnelmesh",
			},
			wantErr: true,
		},
		{
			name: "missing auth token",
			cfg: ServerConfig{
				Listen:       ":8080",
				MeshCIDR:     "10.99.0.0/16",
				DomainSuffix: ".tunnelmesh",
			},
			wantErr: true,
		},
		{
			name: "invalid CIDR",
			cfg: ServerConfig{
				Listen:       ":8080",
				AuthToken:    "token",
				MeshCIDR:     "invalid",
				DomainSuffix: ".tunnelmesh",
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

func TestPeerConfig_Validate(t *testing.T) {
	validConfig := func() PeerConfig {
		return PeerConfig{
			Name:       "testnode",
			Server:     "http://localhost:8080",
			AuthToken:  "token",
			SSHPort:    2222,
			PrivateKey: "/path/to/key",
			TUN: TUNConfig{
				Name: "tun-mesh0",
				MTU:  1400,
			},
			DNS: DNSConfig{
				Enabled:  true,
				Listen:   "127.0.0.53:5353",
				CacheTTL: 60,
			},
		}
	}

	tests := []struct {
		name    string
		modify  func(*PeerConfig)
		wantErr bool
	}{
		{
			name:    "valid config",
			modify:  func(c *PeerConfig) {},
			wantErr: false,
		},
		{
			name:    "missing name",
			modify:  func(c *PeerConfig) { c.Name = "" },
			wantErr: true,
		},
		{
			name:    "missing server",
			modify:  func(c *PeerConfig) { c.Server = "" },
			wantErr: true,
		},
		{
			name:    "invalid ssh port",
			modify:  func(c *PeerConfig) { c.SSHPort = 0 },
			wantErr: true,
		},
		{
			name:    "invalid mtu",
			modify:  func(c *PeerConfig) { c.TUN.MTU = 100 },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(&cfg)
			err := cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadPeerConfig_WithGeolocation(t *testing.T) {
	dir, cleanup := testutil.TempDir(t)
	defer cleanup()

	content := `
name: "geonode"
server: "http://localhost:8080"
auth_token: "token"
geolocation:
  latitude: 51.5074
  longitude: -0.1278
`
	configPath := testutil.TempFile(t, dir, "peer.yaml", content)

	cfg, err := LoadPeerConfig(configPath)
	require.NoError(t, err)

	assert.Equal(t, 51.5074, cfg.Geolocation.Latitude)
	assert.Equal(t, -0.1278, cfg.Geolocation.Longitude)
}

func TestGeolocationConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		geo     GeolocationConfig
		wantErr bool
	}{
		{
			name: "valid location",
			geo: GeolocationConfig{
				Latitude:  40.7128,
				Longitude: -74.0060,
			},
			wantErr: false,
		},
		{
			name: "zero location (null island)",
			geo: GeolocationConfig{
				Latitude:  0,
				Longitude: 0,
			},
			wantErr: false,
		},
		{
			name: "latitude too high",
			geo: GeolocationConfig{
				Latitude:  91.0,
				Longitude: 0,
			},
			wantErr: true,
		},
		{
			name: "latitude too low",
			geo: GeolocationConfig{
				Latitude:  -91.0,
				Longitude: 0,
			},
			wantErr: true,
		},
		{
			name: "longitude too high",
			geo: GeolocationConfig{
				Latitude:  0,
				Longitude: 181.0,
			},
			wantErr: true,
		},
		{
			name: "longitude too low",
			geo: GeolocationConfig{
				Latitude:  0,
				Longitude: -181.0,
			},
			wantErr: true,
		},
		{
			name: "edge case: max valid",
			geo: GeolocationConfig{
				Latitude:  90.0,
				Longitude: 180.0,
			},
			wantErr: false,
		},
		{
			name: "edge case: min valid",
			geo: GeolocationConfig{
				Latitude:  -90.0,
				Longitude: -180.0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.geo.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGeolocationConfig_IsSet(t *testing.T) {
	tests := []struct {
		name   string
		geo    GeolocationConfig
		expect bool
	}{
		{
			name:   "not set (zero values)",
			geo:    GeolocationConfig{},
			expect: false,
		},
		{
			name: "only latitude",
			geo: GeolocationConfig{
				Latitude: 51.5074,
			},
			expect: false,
		},
		{
			name: "only longitude",
			geo: GeolocationConfig{
				Longitude: -0.1278,
			},
			expect: false,
		},
		{
			name: "both set",
			geo: GeolocationConfig{
				Latitude:  51.5074,
				Longitude: -0.1278,
			},
			expect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, tt.geo.IsSet())
		})
	}
}

func TestPeerConfig_ValidateGeolocation(t *testing.T) {
	validConfig := func() PeerConfig {
		return PeerConfig{
			Name:       "testnode",
			Server:     "http://localhost:8080",
			AuthToken:  "token",
			SSHPort:    2222,
			PrivateKey: "/path/to/key",
			TUN: TUNConfig{
				Name: "tun-mesh0",
				MTU:  1400,
			},
		}
	}

	tests := []struct {
		name    string
		modify  func(*PeerConfig)
		wantErr bool
	}{
		{
			name:    "valid without geolocation",
			modify:  func(c *PeerConfig) {},
			wantErr: false,
		},
		{
			name: "valid with geolocation",
			modify: func(c *PeerConfig) {
				c.Geolocation = GeolocationConfig{
					Latitude:  40.7128,
					Longitude: -74.0060,
				}
			},
			wantErr: false,
		},
		{
			name: "invalid geolocation latitude",
			modify: func(c *PeerConfig) {
				c.Geolocation = GeolocationConfig{
					Latitude:  91.0,
					Longitude: 0,
				}
			},
			wantErr: true,
		},
		{
			name: "invalid geolocation longitude",
			modify: func(c *PeerConfig) {
				c.Geolocation = GeolocationConfig{
					Latitude:  0,
					Longitude: 181.0,
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(&cfg)
			err := cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Exit Node Feature Tests

func TestLoadPeerConfig_WithExitNode(t *testing.T) {
	dir, cleanup := testutil.TempDir(t)
	defer cleanup()

	content := `
name: "client-node"
server: "http://localhost:8080"
auth_token: "token"
exit_node: "exit-server"
`
	configPath := testutil.TempFile(t, dir, "peer.yaml", content)

	cfg, err := LoadPeerConfig(configPath)
	require.NoError(t, err)

	assert.Equal(t, "client-node", cfg.Name)
	assert.Equal(t, "exit-server", cfg.ExitNode)
}

func TestLoadPeerConfig_WithAllowExitTraffic(t *testing.T) {
	dir, cleanup := testutil.TempDir(t)
	defer cleanup()

	content := `
name: "exit-server"
server: "http://localhost:8080"
auth_token: "token"
allow_exit_traffic: true
`
	configPath := testutil.TempFile(t, dir, "peer.yaml", content)

	cfg, err := LoadPeerConfig(configPath)
	require.NoError(t, err)

	assert.Equal(t, "exit-server", cfg.Name)
	assert.True(t, cfg.AllowExitTraffic)
}

func TestLoadPeerConfig_ExitNodeDefaults(t *testing.T) {
	dir, cleanup := testutil.TempDir(t)
	defer cleanup()

	// Config without exit fields - they should default to empty/false
	content := `
name: "regular-node"
server: "http://localhost:8080"
auth_token: "token"
`
	configPath := testutil.TempFile(t, dir, "peer.yaml", content)

	cfg, err := LoadPeerConfig(configPath)
	require.NoError(t, err)

	assert.Equal(t, "", cfg.ExitNode, "exit_node should default to empty string")
	assert.False(t, cfg.AllowExitTraffic, "allow_exit_traffic should default to false")
}
