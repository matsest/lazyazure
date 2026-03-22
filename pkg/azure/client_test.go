package azure

import (
	"context"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	// This test will fail if not authenticated, but it tests the constructor
	client, err := NewClient()
	if err != nil {
		t.Skipf("Could not create client (expected if not authenticated): %v", err)
	}

	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	if client.credential == nil {
		t.Fatal("Client credential is nil")
	}
}

func TestVerifyAuthentication(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Skipf("Could not create client: %v", err)
	}

	// Test with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// This will fail if not logged in, but should not hang
	err = client.VerifyAuthentication(ctx)
	if err != nil {
		t.Logf("Authentication verification failed (expected if not logged in): %v", err)
	}
}

func TestVerifyAuthenticationTimeout(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Skipf("Could not create client: %v", err)
	}

	// Test with very short timeout to ensure it doesn't hang
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- client.VerifyAuthentication(ctx)
	}()

	select {
	case err := <-done:
		t.Logf("Authentication returned (may be error): %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("VerifyAuthentication hung for more than 2 seconds")
	}
}
