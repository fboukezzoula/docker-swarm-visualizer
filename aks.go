package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Global variables to store credentials and state
var (
	appID         string
	clientSecret  string
	tenantID      string
	loggedIn      bool // Flag to track login status
	subscriptionID string
	resourceGroup string
	aksName       string
)

// loadCredentials loads authentication credentials from environment variables
func loadCredentials() error {
	appID = os.Getenv("AZURE_APP_ID")
	clientSecret = os.Getenv("AZURE_CLIENT_SECRET")
	tenantID = os.Getenv("AZURE_TENANT_ID")

	if appID == "" || clientSecret == "" || tenantID == "" {
		return fmt.Errorf("missing required environment variables: AZURE_APP_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID")
	}

	fmt.Println("Loaded credentials from environment variables")
	return nil
}

// authenticate logs in to Azure using service principal
func authenticate() error {
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
	loggedIn = true
	return nil
}

// getSubscriptionID retrieves the current Azure subscription ID
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

// setSubscription sets the active Azure subscription
func setSubscription(subscriptionID string) error {
	cmd := exec.Command("az", "account", "set", "--subscription", subscriptionID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set subscription: %w\nOutput:\n%s", err, string(output))
	}

	fmt.Printf("Set active subscription to: %s\n", subscriptionID)
	this.subscriptionID = subID
	return nil
}

// discoverFiles finds all files in a directory (excluding the directory itself)
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
	// Check if required environment variables are set
	if os.Getenv("AZURE_APP_ID") == "" || os.Getenv("AZURE_CLIENT_SECRET") == "" || os.Getenv("AZURE_TENANT_ID") == "" {
		fmt.Println("Error: Required environment variables (AZURE_APP_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID) are not set.")
		os.Exit(1)
	}

	// Authenticate with Azure
	if err := authenticate(); err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}

	// Set a default subscription (or prompt the user to select one)
	defaultSubID := os.Getenv("AZURE_SUBSCRIPTION")
	if defaultSubID == "" {
		fmt.Println("No default subscription set in environment variables.")
		subID, err := getSubscriptionID()
		if err != nil {
			log.Fatalf("Failed to retrieve current subscription: %v", err)
		}
		fmt.Printf("Using current subscription: %s\n", subID)
	} else {
		if err := setSubscription(defaultSubID); err != nil {
			log.Fatalf("Failed to set default subscription: %v", err)
		}
		fmt.Printf("Set active subscription to: %s\n", defaultSubID)
	}

	// Print current status
	subID, err := getSubscriptionID()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Current Status:\n  Logged In: %v\n  Subscription ID: %s\n", loggedIn, subID)

	// Example usage - list AKS clusters in the current subscription
	cmd := exec.Command("az", "aks", "list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to list AKS clusters: %v\nOutput:\n%s", err, string(output))
	}

	fmt.Println("\nAKS Clusters in your subscription:")
	fmt.Println(string(output))
}






*****************************************************


package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"path/filepath"
	// Add these for Azure authentication
	"github.com/Azure/azure-sdk-for-go/azidentity"
	"github.com/Azure/azure-sdk-for-go/azservicebus/rest"
)

var (
	loggedIn bool
	currentSubID string
)

// Authenticate with Azure using service principal
func authenticate() error {
	appID := os.Getenv("AZURE_APP_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")
	tenantID := os.Getenv("AZURE_TENANT_ID")

	if appID == "" || clientSecret == "" || tenantID == "" {
		return fmt.Errorf("required environment variables (AZURE_APP_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID) are not set")
	}

	// Create credentials using the service principal
	creds, err := azidentity.NewServicePrincipalLogin(appID, clientSecret, tenantID)
	if err != nil {
		return fmt.Errorf("failed to create service principal credentials: %w", err)
	}

	loggedIn = true
	currentSubID = os.Getenv("AZURE_SUBSCRIPTION") // Get initial subscription from env

	fmt.Println("Authenticated successfully with Azure!")
	return nil
}

// SetSubscription sets the active Azure subscription
func setSubscription(subscriptionID string) error {
	if !loggedIn {
		return fmt.Errorf("not authenticated - cannot set subscription")
	}

	// Check if the subscription exists first
	_, err := rest.NewSubscriptionsClient(rest.PublicCloud, azidentity.Default())
	if err != nil {
		return fmt.Errorf("failed to create subscriptions client: %w", err)
	}

	fmt.Printf("Setting active subscription to: %s\n", subscriptionID)
	currentSubID = subscriptionID
	return nil
}

// GetSubscription returns the currently set subscription ID
func getSubscription() (string, error) {
	if !loggedIn {
		return "", fmt.Errorf("not authenticated - no subscription is set")
	}

	return currentSubID, nil
}

// ExecuteAzAksCommand executes an Azure AKS command with file attachments
func executeAzAksCommand(command string, resourceGroup, aksName string, files []string) error {
	// Build the az aks command with multiple --file parameters
	var fileParams strings.Builder
	for _, file := range files {
		fileParams.WriteString(" --file \"")
		fileParams.WriteString(file)
		fileParams.WriteString("\"")
	}

	// Include subscription ID in the command
	fullCommand := fmt.Sprintf("az aks command invoke %s --resource-group %s --name %s %s", 
		command, resourceGroup, aksName, fileParams.String())
	
	fmt.Println("Executing:", fullCommand)

	// Execute the command using os/exec
	cmd := exec.Command("bash", "-c", fullCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Error executing command: %v\nOutput:\n%s", err, string(output))
		return fmt.Errorf("command failed: %w", err)
	}

	// Print the output
	fmt.Println(string(output))

	return nil
}

func main() {
	// Set default values from environment variables
	resourceGroup := os.Getenv("RESOURCEGROUP")
	aksName := os.Getenv("AKSNAME")

	if resourceGroup == "" || aksName == "" {
		fmt.Println("Warning: RESOURCEGROUP and AKSNAME environment variables not set.")
		fmt.Println("Using empty values, which will likely result in errors.")
	}

	// Authenticate with Azure
	if err := authenticate(); err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}

	// Set default subscription if not already set
	subscriptionID := os.Getenv("AZURE_SUBSCRIPTION")
	if subscriptionID != "" {
		if err := setSubscription(subscriptionID); err != nil {
			log.Fatalf("Failed to set default subscription: %v", err)
		}
		fmt.Printf("Set active subscription to: %s\n", subscriptionID)
	} else {
		// Prompt user to select a subscription if not set
		subscriptionID, err := getSubscription()
		if err != nil {
			log.Fatalf("Failed to retrieve current subscription: %v", err)
		}
		fmt.Printf("Using current subscription: %s\n", subscriptionID)
	}

	// Print current status
	subID, err := getSubscription()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Current Status:\n  Logged In: %v\n  Subscription ID: %s\n", loggedIn, subID)

	// Define the CLI command
	cmd := &cobra.Command{
		Use:   "xks",
		Short: "CLI for managing Azure Kubernetes Services (AKS)",
		Long:  "Convert xks commands to az aks and execute them.",
	}

	// Create a sub-command for invoking AKS operations
	invokeCmd := &cobra.Command{
		Use:   "invoke [command]",
		Short: "Invoke an Azure Kubernetes Service operation",
		Long:  "Converts xks invoke to az aks and executes the resulting command.",
		Args:  cobra.MinimumNArgs(1), // Require at least one argument (the command)
	}

	// Add positional arguments for the command
	invokeCmd.AddArgument("command", "The AKS operation to perform")

	// Set flags with default values from environment variables
	invokeCmd.Flags().String("resource-group", resourceGroup, "Azure resource group")
	invokeCmd.Flags().String("name", aksName, "AKS cluster name")
    // Add flag for optional subscription override
    invokeCmd.Flags().StringP("subscription", "s", "", "Override the active Azure subscription")

	// Set pre-execute hook to handle command conversion and execution
	invokeCmd.PreExecute = func(cmd *cobra.Command, args []string) error {
		resourceGroup := cmd.Flags().GetString("resource-group")
		aksName := cmd.Flags().GetString("name")
		commandToExecute := strings.Join(args, " ")
        subscriptionOverride := cmd.Flags().GetString("subscription")

		// Use overridden subscription if provided
		if subscriptionOverride != "" {
			if err := setSubscription(subscriptionOverride); err != nil {
				return fmt.Errorf("failed to set subscription: %w", err)
			}
		}

		// Discover all files in the current directory
		files, err := discoverFiles(".")
		if err != nil {
			return fmt.Errorf("failed to discover files: %w", err)
		}

		// Execute the converted command with file attachments
		return executeAzAksCommand(commandToExecute, resourceGroup, aksName, files)
	}

	// Add the sub-command to the main command
	cmd.AddCommand(invokeCmd)

	// Execute the CLI
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

// discoverFiles discovers all files in a directory (excluding hidden files)
func discoverFiles(dir string) ([]string, error) {
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		// Exclude directories
		if !entry.IsDir() {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}

	return files, nil
}












***********************************************************************// How to use:

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
