// Package config handles configuration loading and validation for tunnelmesh.
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ServerConfig holds configuration for the coordination server.
type ServerConfig struct {
	Listen       string `yaml:"listen"`
	AuthToken    string `yaml:"auth_token"`
	MeshCIDR     string `yaml:"mesh_cidr"`
	DomainSuffix string `yaml:"domain_suffix"`
}

// PeerConfig holds configuration for a peer node.
type PeerConfig struct {
	Name       string    `yaml:"name"`
	Server     string    `yaml:"server"`
	AuthToken  string    `yaml:"auth_token"`
	SSHPort    int       `yaml:"ssh_port"`
	PrivateKey string    `yaml:"private_key"`
	TUN        TUNConfig `yaml:"tun"`
	DNS        DNSConfig `yaml:"dns"`
}

// TUNConfig holds configuration for the TUN interface.
type TUNConfig struct {
	Name string `yaml:"name"`
	MTU  int    `yaml:"mtu"`
}

// DNSConfig holds configuration for the local DNS resolver.
type DNSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Listen   string `yaml:"listen"`
	CacheTTL int    `yaml:"cache_ttl"`
}

// LoadServerConfig loads server configuration from a YAML file.
func LoadServerConfig(path string) (*ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	cfg := &ServerConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	// Apply defaults
	if cfg.MeshCIDR == "" {
		cfg.MeshCIDR = "10.99.0.0/16"
	}
	if cfg.DomainSuffix == "" {
		cfg.DomainSuffix = ".mesh"
	}

	return cfg, nil
}

// LoadPeerConfig loads peer configuration from a YAML file.
func LoadPeerConfig(path string) (*PeerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	cfg := &PeerConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	// Apply defaults
	if cfg.SSHPort == 0 {
		cfg.SSHPort = 2222
	}
	if cfg.TUN.Name == "" {
		cfg.TUN.Name = "tun-mesh0"
	}
	if cfg.TUN.MTU == 0 {
		cfg.TUN.MTU = 1400
	}
	if cfg.DNS.Listen == "" {
		cfg.DNS.Listen = "127.0.0.53:5353"
	}
	if cfg.DNS.CacheTTL == 0 {
		cfg.DNS.CacheTTL = 300
	}
	// DNS enabled by default
	if !cfg.DNS.Enabled && cfg.DNS.Listen == "127.0.0.53:5353" {
		cfg.DNS.Enabled = true
	}

	// Expand home directory in private key path
	if strings.HasPrefix(cfg.PrivateKey, "~/") {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			cfg.PrivateKey = filepath.Join(homeDir, cfg.PrivateKey[2:])
		}
	}

	return cfg, nil
}

// Validate checks if the server configuration is valid.
func (c *ServerConfig) Validate() error {
	if c.Listen == "" {
		return fmt.Errorf("listen address is required")
	}
	if c.AuthToken == "" {
		return fmt.Errorf("auth_token is required")
	}
	if c.MeshCIDR != "" {
		_, _, err := net.ParseCIDR(c.MeshCIDR)
		if err != nil {
			return fmt.Errorf("invalid mesh_cidr: %w", err)
		}
	}
	return nil
}

// Validate checks if the peer configuration is valid.
func (c *PeerConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	if c.Server == "" {
		return fmt.Errorf("server is required")
	}
	if c.SSHPort <= 0 || c.SSHPort > 65535 {
		return fmt.Errorf("ssh_port must be between 1 and 65535")
	}
	if c.TUN.MTU < 576 || c.TUN.MTU > 65535 {
		return fmt.Errorf("tun.mtu must be between 576 and 65535")
	}
	return nil
}
