package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tunnelmesh/tunnelmesh/internal/auth"
	"github.com/tunnelmesh/tunnelmesh/internal/context"
)

var (
	userName string
)

func newUserCmd() *cobra.Command {
	userCmd := &cobra.Command{
		Use:   "user",
		Short: "Manage user identity",
		Long: `Manage your TunnelMesh user identity.

Your identity is based on a 3-word recovery phrase that derives an ED25519 keypair.
This identity is portable - you can use it across multiple TunnelMesh networks.

Examples:
  # Create a new identity
  tunnelmesh user setup

  # Recover an existing identity from your recovery phrase
  tunnelmesh user recover

  # Show your current identity
  tunnelmesh user info`,
	}

	// Setup subcommand
	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Create a new user identity",
		Long: `Generate a new 3-word recovery phrase and derive your identity.

IMPORTANT: Store your recovery phrase safely! You will need it to
recover your identity on other devices or if your local data is lost.`,
		RunE: runUserSetup,
	}
	setupCmd.Flags().StringVar(&userName, "name", "", "Optional display name for your identity")
	userCmd.AddCommand(setupCmd)

	// Recover subcommand
	recoverCmd := &cobra.Command{
		Use:   "recover",
		Short: "Recover identity from recovery phrase",
		Long:  `Recover your identity by entering your 3-word recovery phrase.`,
		RunE:  runUserRecover,
	}
	recoverCmd.Flags().StringVar(&userName, "name", "", "Optional display name for your identity")
	userCmd.AddCommand(recoverCmd)

	// Info subcommand
	infoCmd := &cobra.Command{
		Use:   "info",
		Short: "Show current identity",
		Long:  `Display information about your current identity and registration status.`,
		RunE:  runUserInfo,
	}
	userCmd.AddCommand(infoCmd)

	return userCmd
}

func runUserSetup(cmd *cobra.Command, args []string) error {
	identityPath, err := defaultIdentityPath()
	if err != nil {
		return fmt.Errorf("get identity path: %w", err)
	}

	// Check if identity already exists
	if _, err := os.Stat(identityPath); err == nil {
		fmt.Println("An identity already exists at", identityPath)
		fmt.Print("Overwrite? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Generate mnemonic
	mnemonic, err := auth.GenerateMnemonic()
	if err != nil {
		return fmt.Errorf("generate mnemonic: %w", err)
	}

	// Create identity
	identity, err := auth.NewUserIdentity(mnemonic, userName)
	if err != nil {
		return fmt.Errorf("create identity: %w", err)
	}

	// Save identity
	if err := identity.Save(identityPath); err != nil {
		return fmt.Errorf("save identity: %w", err)
	}

	fmt.Println()
	fmt.Println("Your recovery phrase is:")
	fmt.Println()
	fmt.Printf("    %s\n", mnemonic)
	fmt.Println()
	fmt.Println("IMPORTANT: Store this phrase safely! You will need it to recover your identity.")
	fmt.Println()
	fmt.Printf("User ID: %s\n", identity.User.ID)
	if identity.User.Name != "" {
		fmt.Printf("Name:    %s\n", identity.User.Name)
	}
	fmt.Printf("Saved:   %s\n", identityPath)

	return nil
}

func runUserRecover(cmd *cobra.Command, args []string) error {
	identityPath, err := defaultIdentityPath()
	if err != nil {
		return fmt.Errorf("get identity path: %w", err)
	}

	// Check if identity already exists
	if _, err := os.Stat(identityPath); err == nil {
		fmt.Println("An identity already exists at", identityPath)
		fmt.Print("Overwrite? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Prompt for mnemonic
	fmt.Print("Enter your 3-word recovery phrase: ")
	reader := bufio.NewReader(os.Stdin)
	mnemonic, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	mnemonic = strings.TrimSpace(mnemonic)

	// Validate mnemonic
	if err := auth.ValidateMnemonic(mnemonic); err != nil {
		return fmt.Errorf("invalid recovery phrase: %w", err)
	}

	// Create identity
	identity, err := auth.NewUserIdentity(mnemonic, userName)
	if err != nil {
		return fmt.Errorf("create identity: %w", err)
	}

	// Save identity
	if err := identity.Save(identityPath); err != nil {
		return fmt.Errorf("save identity: %w", err)
	}

	fmt.Println()
	fmt.Println("Identity recovered successfully!")
	fmt.Printf("User ID: %s\n", identity.User.ID)
	if identity.User.Name != "" {
		fmt.Printf("Name:    %s\n", identity.User.Name)
	}
	fmt.Printf("Saved:   %s\n", identityPath)

	return nil
}

func runUserInfo(cmd *cobra.Command, args []string) error {
	identityPath, err := defaultIdentityPath()
	if err != nil {
		return fmt.Errorf("get identity path: %w", err)
	}

	identity, err := auth.LoadUserIdentity(identityPath)
	if os.IsNotExist(err) {
		fmt.Println("No identity found. Run 'tunnelmesh user setup' to create one.")
		return nil
	}
	if err != nil {
		return fmt.Errorf("load identity: %w", err)
	}

	fmt.Printf("User ID:    %s\n", identity.User.ID)
	if identity.User.Name != "" {
		fmt.Printf("Name:       %s\n", identity.User.Name)
	}
	fmt.Printf("Public Key: %s\n", identity.User.PublicKey)
	fmt.Printf("Created:    %s\n", identity.User.CreatedAt.Format("2006-01-02 15:04:05 UTC"))

	// Show registration status for active context
	store, err := context.Load()
	if err != nil {
		return nil // Ignore context errors
	}

	activeCtx := store.GetActive()
	if activeCtx != nil {
		fmt.Println()
		fmt.Printf("Active Context: %s\n", activeCtx.Name)
		if activeCtx.IsRegistered() {
			fmt.Printf("Registered:     Yes (ID: %s)\n", activeCtx.UserID)
			// Try to load registration for more details
			if activeCtx.RegistrationPath != "" {
				if reg, err := auth.LoadRegistration(activeCtx.RegistrationPath); err == nil {
					if len(reg.Roles) > 0 {
						fmt.Printf("Roles:          %s\n", strings.Join(reg.Roles, ", "))
					}
				}
			}
		} else {
			fmt.Println("Registered:     No")
			fmt.Println("Run 'tunnelmesh user register' to register with this mesh.")
		}
	}

	return nil
}

func defaultIdentityPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".tunnelmesh", "user.json"), nil
}
