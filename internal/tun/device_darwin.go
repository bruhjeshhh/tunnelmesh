//go:build darwin

package tun

import "github.com/songgao/water"

func configurePlatformTUN(cfg *water.Config, name, address string) {
	// macOS uses utun devices - water will auto-assign a utun number
	// The Name field exists but setting a custom name is not supported
}
