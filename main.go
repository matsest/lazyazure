package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jesseduffield/gocui"
	"github.com/matsest/lazyazure/pkg/azure"
	"github.com/matsest/lazyazure/pkg/demo"
	"github.com/matsest/lazyazure/pkg/gui"
	"github.com/matsest/lazyazure/pkg/utils"
)

// printVersion prints version information
func printVersion() {
	// Shorten commit for display
	displayCommit := commit
	if len(displayCommit) > 7 {
		displayCommit = displayCommit[:7]
	}

	fmt.Printf("lazyazure %s (%s)\n", version, displayCommit)
}

// Version info - set by GoReleaser during build
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// VersionInfo holds version information for the GUI
type VersionInfo struct {
	Version string
	Commit  string
	Date    string
}

// GetVersionInfo returns the current version information
func GetVersionInfo() VersionInfo {
	return VersionInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}
}

// GitHubRelease represents a GitHub release API response
type GitHubRelease struct {
	TagName string `json:"tag_name"`
}

// checkUpdate checks for updates from GitHub releases
func checkUpdate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/matsest/lazyazure/releases/latest", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "lazyazure")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch release: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}

	var release GitHubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return fmt.Errorf("failed to parse response: %v", err)
	}

	// Print current version
	displayCommit := commit
	if len(displayCommit) > 7 {
		displayCommit = displayCommit[:7]
	}
	fmt.Printf("Current version: lazyazure %s (%s)\n", version, displayCommit)
	fmt.Printf("Latest version:  %s\n", release.TagName)

	// Check if development build
	isDev := isDevelopmentBuild()
	if isDev {
		fmt.Println()
		fmt.Println("Note: This is a development build. Skipping version comparison.")
		return nil
	}

	// Compare versions (normalize by stripping "v" prefix)
	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(version, "v")
	if latestVersion != currentVersion {
		fmt.Println()
		fmt.Printf("Update available! You are running %s, latest is %s\n", version, release.TagName)
		fmt.Println()
		fmt.Println("To update, download from: https://github.com/matsest/lazyazure/releases/latest")
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("You are running the latest version.")
	return nil
}

// isDevelopmentBuild checks if this is a development/non-release build
func isDevelopmentBuild() bool {
	if version == "dev" {
		return true
	}
	if strings.Contains(version, "dirty") {
		return true
	}
	if strings.Contains(version, "-") {
		parts := strings.Split(version, "-")
		for i, part := range parts {
			if i == 0 {
				continue
			}
			if len(part) > 1 && part[0] == 'g' {
				return true
			}
		}
	}
	return false
}

func main() {
	// Handle version command
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-v") {
		printVersion()
		os.Exit(0)
	}

	// Handle check-update command
	if len(os.Args) > 1 && os.Args[1] == "--check-update" {
		if err := checkUpdate(); err != nil {
			fmt.Fprintf(os.Stderr, "Error checking for updates: %v\n", err)
			os.Exit(2)
		}
		os.Exit(0)
	}
	// Initialize logger if LAZYAZURE_DEBUG is set
	if utils.IsDebugEnabled() {
		if err := utils.InitLogger(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
			os.Exit(1)
		}
		defer utils.CloseLogger()
		utils.Log("Starting LazyAzure with debug logging enabled...")
	}

	var azureClient gui.AzureClient
	var clientFactory gui.AzureClientFactory

	// Check if demo mode is enabled
	demoMode := os.Getenv("LAZYAZURE_DEMO")
	if demoMode == "1" || demoMode == "2" {
		utils.Log("Demo mode enabled (mode=%s)", demoMode)

		// Create demo client with specified mode
		demoClient := demo.NewClientWithMode(demoMode)
		azureClient = demoClient
		clientFactory = demoClient

	} else {
		// Create real Azure client
		client, err := azure.NewClient()
		if err != nil {
			utils.Log("ERROR: Failed to create Azure client: %v", err)
			log.Fatalf("Failed to create Azure client: %v", err)
		}
		utils.Log("Azure client created successfully")

		// Verify authentication
		ctx := context.Background()
		if err := client.VerifyAuthentication(ctx); err != nil {
			utils.Log("ERROR: Authentication failed: %v", err)
			fmt.Fprintf(os.Stderr, "Authentication failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "Please ensure you're logged in with 'az login'\n")
			os.Exit(1)
		}
		utils.Log("Authentication verified successfully")

		azureClient = client
		clientFactory = azure.NewClientFactory(client)
	}

	// Create and run GUI
	versionInfo := gui.VersionInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}
	g, err := gui.NewGui(azureClient, clientFactory, versionInfo)
	if err != nil {
		utils.Log("ERROR: Failed to create GUI: %v", err)
		log.Fatalf("Failed to create GUI: %v", err)
	}
	utils.Log("GUI created successfully")

	utils.Log("Starting GUI main loop...")
	runErr := g.Run()
	utils.Log("GUI Run returned: %v", runErr)

	// Check if it's a quit error - gocui.ErrQuit or any error containing "quit"
	if runErr != nil {
		if runErr.Error() == "quit" || runErr.Error() == gocui.ErrQuit.Error() {
			utils.Log("Normal quit - exiting cleanly")
			os.Exit(0)
		} else {
			utils.Log("ERROR: GUI error: %v", runErr)
			log.Fatalf("GUI error: %v", runErr)
		}
	}

	utils.Log("Application exiting normally")
	os.Exit(0)
}
