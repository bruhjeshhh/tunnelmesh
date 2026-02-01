// tunnelmesh is the peer daemon for the tunnelmesh network.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/tunnelmesh/tunnelmesh/internal/config"
	"github.com/tunnelmesh/tunnelmesh/internal/coord"
	meshdns "github.com/tunnelmesh/tunnelmesh/internal/dns"
	"github.com/tunnelmesh/tunnelmesh/pkg/proto"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

var (
	cfgFile   string
	logLevel  string
	serverURL string
	authToken string
	nodeName  string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "tunnelmesh",
		Short: "TunnelMesh peer daemon",
		Long: `TunnelMesh creates encrypted P2P tunnels between peers using SSH.

It connects to a coordination server for peer discovery and establishes
direct SSH tunnels to other peers in the mesh network.`,
	}

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info", "log level")

	// Join command
	joinCmd := &cobra.Command{
		Use:   "join",
		Short: "Join the mesh network",
		Long:  "Register with the coordination server and start the mesh daemon",
		RunE:  runJoin,
	}
	joinCmd.Flags().StringVarP(&serverURL, "server", "s", "", "coordination server URL")
	joinCmd.Flags().StringVarP(&authToken, "token", "t", "", "authentication token")
	joinCmd.Flags().StringVarP(&nodeName, "name", "n", "", "node name")
	rootCmd.AddCommand(joinCmd)

	// Status command
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show mesh status",
		RunE:  runStatus,
	}
	rootCmd.AddCommand(statusCmd)

	// Peers command
	peersCmd := &cobra.Command{
		Use:   "peers",
		Short: "List mesh peers",
		RunE:  runPeers,
	}
	rootCmd.AddCommand(peersCmd)

	// Resolve command
	resolveCmd := &cobra.Command{
		Use:   "resolve <hostname>",
		Short: "Resolve a mesh hostname",
		Args:  cobra.ExactArgs(1),
		RunE:  runResolve,
	}
	rootCmd.AddCommand(resolveCmd)

	// Leave command
	leaveCmd := &cobra.Command{
		Use:   "leave",
		Short: "Leave the mesh network",
		RunE:  runLeave,
	}
	rootCmd.AddCommand(leaveCmd)

	// Version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("tunnelmesh %s\n", Version)
			fmt.Printf("  Commit:     %s\n", Commit)
			fmt.Printf("  Build Time: %s\n", BuildTime)
		},
	}
	rootCmd.AddCommand(versionCmd)

	// Init command - generate keys
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize tunnelmesh (generate keys)",
		RunE:  runInit,
	}
	rootCmd.AddCommand(initCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	setupLogging()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	keyDir := filepath.Join(homeDir, ".tunnelmesh")
	keyPath := filepath.Join(keyDir, "id_ed25519")

	// Check if key already exists
	if _, err := os.Stat(keyPath); err == nil {
		log.Info().Str("path", keyPath).Msg("keys already exist")
		return nil
	}

	if err := config.GenerateKeyPair(keyPath); err != nil {
		return fmt.Errorf("generate keys: %w", err)
	}

	log.Info().Str("path", keyPath).Msg("keys generated")
	log.Info().Str("path", keyPath+".pub").Msg("public key")
	return nil
}

func runJoin(cmd *cobra.Command, args []string) error {
	setupLogging()

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// Override with flags
	if serverURL != "" {
		cfg.Server = serverURL
	}
	if authToken != "" {
		cfg.AuthToken = authToken
	}
	if nodeName != "" {
		cfg.Name = nodeName
	}

	if cfg.Server == "" || cfg.AuthToken == "" || cfg.Name == "" {
		return fmt.Errorf("server, token, and name are required")
	}

	// Ensure keys exist
	signer, err := config.EnsureKeyPairExists(cfg.PrivateKey)
	if err != nil {
		return fmt.Errorf("load keys: %w", err)
	}

	pubKeyFP := config.GetPublicKeyFingerprint(signer.PublicKey())
	log.Info().Str("fingerprint", pubKeyFP).Msg("using SSH key")

	// Get local IPs
	publicIPs, privateIPs := proto.GetLocalIPs()
	log.Debug().
		Strs("public", publicIPs).
		Strs("private", privateIPs).
		Msg("detected local IPs")

	// Connect to coordination server
	client := coord.NewClient(cfg.Server, cfg.AuthToken)

	resp, err := client.Register(cfg.Name, pubKeyFP, publicIPs, privateIPs, cfg.SSHPort)
	if err != nil {
		return fmt.Errorf("register with server: %w", err)
	}

	log.Info().
		Str("mesh_ip", resp.MeshIP).
		Str("mesh_cidr", resp.MeshCIDR).
		Str("domain", resp.Domain).
		Msg("joined mesh network")

	// Start DNS resolver if enabled
	var resolver *meshdns.Resolver
	if cfg.DNS.Enabled {
		resolver = meshdns.NewResolver(resp.Domain, cfg.DNS.CacheTTL)

		// Initial DNS sync
		if err := syncDNS(client, resolver); err != nil {
			log.Warn().Err(err).Msg("failed to sync DNS")
		}

		go func() {
			if err := resolver.ListenAndServe(cfg.DNS.Listen); err != nil {
				log.Error().Err(err).Msg("DNS server error")
			}
		}()
		log.Info().Str("listen", cfg.DNS.Listen).Msg("DNS server started")
	}

	// Start heartbeat loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go heartbeatLoop(ctx, client, cfg.Name, pubKeyFP, resolver)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Info().Msg("shutting down...")

	// Deregister
	if err := client.Deregister(cfg.Name); err != nil {
		log.Warn().Err(err).Msg("failed to deregister")
	} else {
		log.Info().Msg("deregistered from mesh")
	}

	if resolver != nil {
		resolver.Shutdown()
	}

	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	setupLogging()
	fmt.Println("Status: Not implemented yet")
	fmt.Println("Use 'tunnelmesh peers' to list connected peers")
	return nil
}

func runPeers(cmd *cobra.Command, args []string) error {
	setupLogging()

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	client := coord.NewClient(cfg.Server, cfg.AuthToken)
	peers, err := client.ListPeers()
	if err != nil {
		return fmt.Errorf("list peers: %w", err)
	}

	if len(peers) == 0 {
		fmt.Println("No peers in mesh")
		return nil
	}

	fmt.Printf("%-20s %-15s %-20s %s\n", "NAME", "MESH IP", "PUBLIC IP", "LAST SEEN")
	fmt.Println("-------------------- --------------- -------------------- --------------------")

	for _, p := range peers {
		publicIP := "-"
		if len(p.PublicIPs) > 0 {
			publicIP = p.PublicIPs[0]
		}
		lastSeen := p.LastSeen.Format("2006-01-02 15:04:05")
		fmt.Printf("%-20s %-15s %-20s %s\n", p.Name, p.MeshIP, publicIP, lastSeen)
	}

	return nil
}

func runResolve(cmd *cobra.Command, args []string) error {
	setupLogging()

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	hostname := args[0]

	client := coord.NewClient(cfg.Server, cfg.AuthToken)
	records, err := client.GetDNSRecords()
	if err != nil {
		return fmt.Errorf("get DNS records: %w", err)
	}

	for _, r := range records {
		if r.Hostname == hostname {
			fmt.Printf("%s -> %s\n", hostname, r.MeshIP)
			return nil
		}
	}

	return fmt.Errorf("hostname not found: %s", hostname)
}

func runLeave(cmd *cobra.Command, args []string) error {
	setupLogging()

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	client := coord.NewClient(cfg.Server, cfg.AuthToken)
	if err := client.Deregister(cfg.Name); err != nil {
		return fmt.Errorf("deregister: %w", err)
	}

	log.Info().Msg("left mesh network")
	return nil
}

func loadConfig() (*config.PeerConfig, error) {
	if cfgFile != "" {
		return config.LoadPeerConfig(cfgFile)
	}

	// Try default locations
	homeDir, _ := os.UserHomeDir()
	defaults := []string{
		filepath.Join(homeDir, ".tunnelmesh", "config.yaml"),
		"tunnelmesh.yaml",
		"peer.yaml",
	}

	for _, path := range defaults {
		if _, err := os.Stat(path); err == nil {
			return config.LoadPeerConfig(path)
		}
	}

	// Return empty config with defaults
	return &config.PeerConfig{
		SSHPort:    2222,
		PrivateKey: filepath.Join(homeDir, ".tunnelmesh", "id_ed25519"),
		TUN: config.TUNConfig{
			Name: "tun-mesh0",
			MTU:  1400,
		},
		DNS: config.DNSConfig{
			Enabled:  true,
			Listen:   "127.0.0.53:5353",
			CacheTTL: 300,
		},
	}, nil
}

func heartbeatLoop(ctx context.Context, client *coord.Client, name, pubKey string, resolver *meshdns.Resolver) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := client.Heartbeat(name, pubKey); err != nil {
				log.Warn().Err(err).Msg("heartbeat failed")
				continue
			}

			// Sync DNS records
			if resolver != nil {
				if err := syncDNS(client, resolver); err != nil {
					log.Warn().Err(err).Msg("DNS sync failed")
				}
			}
		}
	}
}

func syncDNS(client *coord.Client, resolver *meshdns.Resolver) error {
	records, err := client.GetDNSRecords()
	if err != nil {
		return err
	}

	recordMap := make(map[string]string, len(records))
	for _, r := range records {
		recordMap[r.Hostname] = r.MeshIP
	}

	resolver.UpdateRecords(recordMap)
	return nil
}

func setupLogging() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
}
