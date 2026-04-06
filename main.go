package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matsest/lazyazure/pkg/azure"
	"github.com/matsest/lazyazure/pkg/demo"
	"github.com/matsest/lazyazure/pkg/gui"
	"github.com/matsest/lazyazure/pkg/tui"
	"github.com/matsest/lazyazure/pkg/utils"
)

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

// CLIArgs holds the parsed command-line arguments
type CLIArgs struct {
	ShowVersion bool
	CheckUpdate bool
	ShowHelp    bool
}

// parseArgs parses command-line arguments and returns a CLIArgs struct
func parseArgs(args []string) (CLIArgs, error) {
	var result CLIArgs

	if len(args) > 1 {
		switch args[1] {
		case "-h", "--help":
			result.ShowHelp = true
		case "-v", "--version":
			result.ShowVersion = true
		case "--check-update":
			result.CheckUpdate = true
		default:
			return result, fmt.Errorf("unknown flag: %s", args[1])
		}
	}

	return result, nil
}

// printVersion prints version information
func printVersion(version, commit string) {
	// Shorten commit for display
	displayCommit := commit
	if len(displayCommit) > 7 {
		displayCommit = displayCommit[:7]
	}

	fmt.Printf("lazyazure %s (%s)\n", version, displayCommit)
}

// printHelp prints help information
func printHelp() {
	fmt.Println("LazyAzure - A TUI application for viewing Azure resources")
	fmt.Println()
	fmt.Println("Usage: lazyazure [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -h, --help          Show this help message")
	fmt.Println("  -v, --version       Show version information")
	fmt.Println("      --check-update  Check for available updates")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  LAZYAZURE_DEBUG=1       Enable debug logging (logs to ~/.lazyazure/debug.log)")
	fmt.Println("  LAZYAZURE_DEMO=1        Run with mock data (small dataset)")
	fmt.Println("  LAZYAZURE_DEMO=2        Run with mock data (large dataset)")
	fmt.Println("  LAZYAZURE_CACHE_SIZE    Cache size: small (100/500), medium (300/1500, default), large (600/3000)")
	fmt.Println()
	fmt.Println("For more information, visit: https://github.com/matsest/lazyazure")
}

// GitHubRelease represents a GitHub release API response
type GitHubRelease struct {
	TagName string `json:"tag_name"`
}

// checkUpdate checks for updates from GitHub releases
func checkUpdate(version, commit string, httpClient *http.Client, apiURL string) (exitCode int, err error) {
	if apiURL == "" {
		apiURL = "https://api.github.com/repos/matsest/lazyazure/releases/latest"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return 2, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "lazyazure")

	resp, err := httpClient.Do(req)
	if err != nil {
		return 2, fmt.Errorf("failed to fetch release: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 2, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 2, fmt.Errorf("failed to read response: %v", err)
	}

	var release GitHubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return 2, fmt.Errorf("failed to parse response: %v", err)
	}

	// Print current version
	displayCommit := commit
	if len(displayCommit) > 7 {
		displayCommit = displayCommit[:7]
	}
	fmt.Printf("Current version: lazyazure %s (%s)\n", version, displayCommit)
	fmt.Printf("Latest version:  %s\n", release.TagName)

	// Check if development build
	isDev := isDevelopmentBuild(version)
	if isDev {
		fmt.Println()
		fmt.Println("Note: This is a development build. Skipping version comparison.")
		return 0, nil
	}

	// Compare versions (normalize by stripping "v" prefix)
	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(version, "v")
	if latestVersion != currentVersion {
		fmt.Println()
		fmt.Printf("Update available! You are running %s, latest is %s\n", version, release.TagName)
		fmt.Println()
		fmt.Println("To update, download from: https://github.com/matsest/lazyazure/releases/latest")
		return 1, nil
	}

	fmt.Println()
	fmt.Println("You are running the latest version.")
	return 0, nil
}

// isDevelopmentBuild checks if this is a development/non-release build
func isDevelopmentBuild(version string) bool {
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

// runCLI handles CLI commands (--version, --check-update, --help) and returns exit code
func runCLI(args CLIArgs, version, commit string, httpClient *http.Client, apiURL string) int {
	if args.ShowHelp {
		printHelp()
		return 0
	}

	if args.ShowVersion {
		printVersion(version, commit)
		return 0
	}

	if args.CheckUpdate {
		exitCode, err := checkUpdate(version, commit, httpClient, apiURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error checking for updates: %v\n", err)
			return 2
		}
		return exitCode
	}

	return -1 // -1 means no CLI command handled, should run app
}

// runApp initializes and runs the GUI application
func runApp(version, commit, date string) int {
	// Initialize logger if LAZYAZURE_DEBUG is set
	if utils.IsDebugEnabled() {
		if err := utils.InitLogger(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
			return 1
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
			fmt.Fprintf(os.Stderr, "Failed to create Azure client: %v\n", err)
			return 1
		}
		utils.Log("Azure client created successfully")

		// Verify authentication
		ctx := context.Background()
		if err := client.VerifyAuthentication(ctx); err != nil {
			utils.Log("ERROR: Authentication failed: %v", err)
			fmt.Fprintf(os.Stderr, "Authentication failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "Please ensure you're logged in with 'az login'\n")
			return 1
		}
		utils.Log("Authentication verified successfully")

		azureClient = client
		clientFactory = azure.NewClientFactory(client)
	}

	// Create and run TUI
	versionInfo := gui.VersionInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}

	isDemo := demoMode == "1" || demoMode == "2"
	m := tui.NewModel(azureClient, clientFactory, versionInfo, isDemo)
	p := tea.NewProgram(m, tea.WithAltScreen())

	utils.Log("Starting TUI main loop...")
	if _, err := p.Run(); err != nil {
		utils.Log("ERROR: TUI error: %v", err)
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		return 1
	}

	utils.Log("Application exiting normally")
	return 0
}

func main() {
	// Parse command-line arguments
	args, err := parseArgs(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Usage: lazyazure [options]\n")
		fmt.Fprintf(os.Stderr, "Run 'lazyazure --help' for more information.\n")
		os.Exit(1)
	}

	// Handle CLI commands
	exitCode := runCLI(args, version, commit, &http.Client{Timeout: 10 * time.Second}, "")
	if exitCode >= 0 {
		os.Exit(exitCode)
	}

	// Run the GUI application
	os.Exit(runApp(version, commit, date))
}
