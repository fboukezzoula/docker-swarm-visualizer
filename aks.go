// How to use:

// Save the code as main.go
// Run go mod init xks-cli (or your preferred module name)
// Run go get github.com/spf13/cobra@latest
// Build: go build -o xks main.go
// Set environment variables:
// export RESOURCEGROUP="your-resource-group"
// export AKSNAME="your-aks-cluster-name"
// This approach allows users to apply complex Kubernetes configurations with multiple files in a single command, streamlining their workflow when deploying applications to Azure AKS.

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	// "github.com/spf13/cobra"     # go install github.com/spf13/cobra
)

// Global variables to store credentials - populated by loadCredentials()
var (
	appID         string
	clientSecret  string
	tenantID      string
	loggedIn      bool // Flag to indicate if the user is logged in
	resourceGroup string
	aksName       string
)

// LoadCredentials reads authentication credentials from environment variables
func loadCredentials() error {
	appID := os.Getenv("AZURE_APP_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")
	tenantID := os.Getenv("AZURE_TENANT_ID")

	if appID == "" || clientSecret == "" || tenantID == "" {
		return fmt.Errorf("missing required environment variables: AZURE_APP_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID")
	}

	// Set global variables
	appID = appID
	clientSecret = clientSecret
	tenantID = tenantID

	fmt.Println("Loaded credentials from environment variables")
	return nil
}

// Authenticate logs the user in to Azure using service principal
func authenticate() error {
	// Load credentials before authentication
	if err := loadCredentials(); err != nil {
		return fmt.Errorf("failed to load credentials: %w", err)
	}

	fmt.Println("Authenticating...")
	cmd := exec.Command("az", "login", "--service-principal", "-i")
	cmd.Env = append(os.Environ(),
		"AZURE_APP_ID="+appID,
		"AZURE_CLIENT_SECRET="+clientSecret,
		"AZURE_TENANT_ID="+tenantID)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("authentication failed: %w\nOutput:\n%s", err, string(output))
	}

	fmt.Println("Authenticated successfully!")
	loggedIn = true // Set the login status to true
	return nil
}

// GetSubscriptionID retrieves the current Azure subscription ID
func getSubscriptionID() (string, error) {
	cmd := exec.Command("az", "account", "show")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get subscription ID: %w\nOutput:\n%s", err, string(output))
	}

	// Parse the output to extract the subscription ID
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "id:") {
			subID := strings.TrimSpace(strings.Split(line, ":")[1])
			return subID, nil
		}
	}

	return "", fmt.Errorf("unable to extract subscription ID from output")
}

// SetSubscription sets the active Azure subscription
func setSubscription(subscriptionID string) error {
	cmd := exec.Command("az", "account", "set", "--subscription", subscriptionID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set subscription: %w\nOutput:\n%s", err, string(output))
	}

	fmt.Println("Set active subscription to:", subscriptionID)
	return nil
}

// DiscoverFiles finds all files in a directory
func discoverFiles(dirPath string) ([]string, error) {
	files := []string{}
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && path != dirPath+string(os.PathSeparator) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to discover files: %w", err)
	}
	return files, nil
}

func main() {
	// Create a new cobra command root
	cmd := &cobra.Command{
		Use:   "xks",
		Short: "Manage Azure Kubernetes Services from the CLI",
		Long:  `A tool to simplify managing Azure AKS clusters with features like automatic file attachment and subscription management.`,
	}

	// Authentication command
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate with Azure using service principal",
		Run: func(cmd *cobra.Command, args []string) error {
			if err := authenticate(); err != nil {
				return fmt.Errorf("authentication failed: %w", err)
			}
			fmt.Println("Authenticated successfully!")
			return nil
		},
	}

	// Subscription set command
	subscriptionSetCmd := &cobra.Command{
		Use:   "sub [subscription-id]",
		Short: "Set the active Azure subscription",
		Long:  "Sets the current subscription to use for all commands.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) error {
			subID := args[0]
			if err := setSubscription(subID); err != nil {
				return fmt.Errorf("failed to set subscription: %w", err)
			}
			fmt.Println("Set active subscription to:", subID)
			return nil
		},
	}

	// Status command
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show current configuration and status",
		Run: func(cmd *cobra.Command, args []string) error {
			if !loggedIn {
				return fmt.Errorf("not logged in - run 'xks auth' first")
			}

			subID, err := getSubscriptionID()
			if err != nil {
				return err
			}

			fmt.Println("Current Status:")
			fmt.Printf("  Logged In: %v\n", loggedIn)
			fmt.Printf("  Subscription ID: %s\n", subID)
			return nil
		},
	}

	// Invoke command with file discovery
	invokeCmd := &cobra.Command{
		Use:   "invoke [command]",
		Short: "Execute Azure AKS commands with automatic file attachment",
		Long:  `Invoke an AKS command and automatically attach all files in the current directory as input.`,
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) error {
			command := strings.Join(args, " ")
			files, err := discoverFiles(".")
			if err != nil {
				return fmt.Errorf("failed to discover files: %w", err)
			}

			// Add file attachments to the command
			var fileParams string
			for _, file := range files {
				fileParams += " --file \"" + file + "\""
			}

			fullCommand := "az aks command invoke " + resourceGroup + " " + aksName + " " + command + fileParams
			fmt.Println(fullCommand)

			cmdExec := exec.Command("bash", "-c", fullCommand)
			output, err := cmdExec.CombinedOutput()
			if err != nil {
				log.Printf("Error executing command: %v\nOutput:\n%s", err, string(output))
				return fmt.Errorf("command failed: %w", err)
			}

			fmt.Println(string(output))
			return nil
		},
	}

	// Add the sub-commands to the root command
	cmd.AddCommand(authCmd, subscriptionSetCmd, statusCmd, invokeCmd)

	// Execute the command
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
