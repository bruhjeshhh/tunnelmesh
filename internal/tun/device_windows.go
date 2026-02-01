//go:build windows

package tun

import "github.com/songgao/water"

func configurePlatformTUN(cfg *water.Config, name, address string) {
	cfg.InterfaceName = name
	// Network is required for TUN on Windows to generate ARP responses
	cfg.Network = address
}
