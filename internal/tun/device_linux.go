//go:build linux

package tun

import "github.com/songgao/water"

func configurePlatformTUN(cfg *water.Config, name, address string) {
	cfg.Name = name
}
