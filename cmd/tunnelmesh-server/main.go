// tunnelmesh-server is the coordination server for the tunnelmesh network.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/tunnelmesh/tunnelmesh/internal/config"
	"github.com/tunnelmesh/tunnelmesh/internal/coord"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

var (
	cfgFile  string
	logLevel string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "tunnelmesh-server",
		Short: "TunnelMesh coordination server",
		Long: `TunnelMesh server manages peer registration and discovery for the mesh network.

It provides a REST API for peers to:
- Register and deregister from the mesh
- Discover other peers
- Get DNS records for .mesh hostnames`,
		RunE: runServer,
	}

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info", "log level (debug, info, warn, error)")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("tunnelmesh-server %s\n", Version)
			fmt.Printf("  Commit:     %s\n", Commit)
			fmt.Printf("  Build Time: %s\n", BuildTime)
		},
	}
	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runServer(cmd *cobra.Command, args []string) error {
	setupLogging()

	if cfgFile == "" {
		return fmt.Errorf("config file required (use --config)")
	}

	cfg, err := config.LoadServerConfig(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	srv, err := coord.NewServer(cfg)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info().Msg("shutting down...")
		os.Exit(0)
	}()

	log.Info().
		Str("listen", cfg.Listen).
		Str("mesh_cidr", cfg.MeshCIDR).
		Str("domain", cfg.DomainSuffix).
		Msg("starting tunnelmesh server")

	return srv.ListenAndServe()
}

func setupLogging() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Use pretty console output
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
}
