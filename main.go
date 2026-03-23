package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jesseduffield/gocui"
	"github.com/matsest/lazyazure/pkg/azure"
	"github.com/matsest/lazyazure/pkg/demo"
	"github.com/matsest/lazyazure/pkg/gui"
	"github.com/matsest/lazyazure/pkg/utils"
)

func main() {
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
	if os.Getenv("LAZYAZURE_DEMO") == "1" {
		fmt.Println("Demo mode enabled - using mock Azure data...")
		utils.Log("Demo mode enabled")

		// Create demo client
		demoClient := demo.NewClient()
		azureClient = demoClient
		clientFactory = demoClient

		fmt.Println("Demo mode active! Starting GUI...")
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

		fmt.Println("Authentication successful! Starting GUI...")

		azureClient = client
		clientFactory = azure.NewClientFactory(client)
	}

	// Create and run GUI
	g, err := gui.NewGui(azureClient, clientFactory)
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
			fmt.Println("Goodbye!")
			os.Exit(0)
		} else {
			utils.Log("ERROR: GUI error: %v", runErr)
			log.Fatalf("GUI error: %v", runErr)
		}
	}

	utils.Log("Application exiting normally")
	fmt.Println("Goodbye!")
	os.Exit(0)
}
