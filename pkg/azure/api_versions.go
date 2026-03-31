package azure

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/matsest/lazyazure/pkg/utils"
)

//go:embed api_versions_curated.json
var curatedAPIVersionsFS embed.FS

// CuratedAPIVersions represents the structure of api_versions_curated.json
type CuratedAPIVersions struct {
	LastUpdated   string            `json:"lastUpdated"`
	Source        string            `json:"source"`
	ResourceCount int               `json:"resourceCount"`
	APIVersions   map[string]string `json:"apiVersions"`
}

// init loads curated API versions at startup
func init() {
	loadCuratedAPIVersions()
}

// loadCuratedAPIVersions loads pre-populated API versions from embedded JSON
func loadCuratedAPIVersions() {
	data, err := curatedAPIVersionsFS.ReadFile("api_versions_curated.json")
	if err != nil {
		utils.Log("APIVersionCache: Failed to load curated versions: %v", err)
		return
	}

	var curated CuratedAPIVersions
	if err := json.Unmarshal(data, &curated); err != nil {
		utils.Log("APIVersionCache: Failed to parse curated versions: %v", err)
		return
	}

	// Populate global cache with curated versions
	globalAPICacheMux.Lock()
	for resourceType, version := range curated.APIVersions {
		// Store as single-item slice (format expected by cache)
		globalAPICache[resourceType] = []string{version}
	}
	globalAPICacheMux.Unlock()

	utils.Log("APIVersionCache: Loaded %d curated API versions from %s (last updated: %s)",
		curated.ResourceCount, curated.Source, curated.LastUpdated)
}

// globalAPICache is a singleton cache shared across all APIVersionCache instances
var (
	globalAPICache    = make(map[string][]string) // resourceType -> API versions
	globalAPICacheMux sync.RWMutex
)

// APIVersionCache caches API versions for resource providers
type APIVersionCache struct {
	client         *armresources.ProvidersClient
	subscriptionID string
}

// NewAPIVersionCache creates a new API version cache client
func NewAPIVersionCache(client *Client, subscriptionID string) (*APIVersionCache, error) {
	providersClient, err := armresources.NewProvidersClient(subscriptionID, client.Credential(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create providers client: %w", err)
	}

	return &APIVersionCache{
		client:         providersClient,
		subscriptionID: subscriptionID,
	}, nil
}

// GetLatestAPIVersion returns the latest API version for a resource type
func (c *APIVersionCache) GetLatestAPIVersion(ctx context.Context, resourceType string) (string, error) {
	// Parse resource type to get provider namespace
	// Format: Microsoft.Provider/resourceType
	parts := strings.Split(resourceType, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid resource type format: %s", resourceType)
	}

	providerNamespace := parts[0]
	typeName := parts[1]

	// Check cache first (thread-safe read)
	globalAPICacheMux.RLock()
	if versions, ok := globalAPICache[resourceType]; ok && len(versions) > 0 {
		globalAPICacheMux.RUnlock()
		// Check if this was pre-loaded (curated) or fetched at runtime
		version := versions[0]
		utils.Log("APIVersionCache: CACHE HIT for %s (version: %s)", resourceType, version)
		return version, nil
	}
	globalAPICacheMux.RUnlock()
	utils.Log("APIVersionCache: CACHE MISS for %s (will fetch from Azure)", resourceType)

	// Fetch from Azure
	resp, err := c.client.Get(ctx, providerNamespace, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get provider %s: %w", providerNamespace, err)
	}

	// Find the resource type and its API versions
	for _, rt := range resp.ResourceTypes {
		if rt.ResourceType != nil && *rt.ResourceType == typeName {
			if len(rt.APIVersions) > 0 {
				// Cache the versions (thread-safe write)
				versions := make([]string, len(rt.APIVersions))
				for i, v := range rt.APIVersions {
					versions[i] = *v
				}
				globalAPICacheMux.Lock()
				globalAPICache[resourceType] = versions
				globalAPICacheMux.Unlock()
				// Return first (latest) version
				return versions[0], nil
			}
		}
	}

	return "", fmt.Errorf("no API versions found for resource type %s", resourceType)
}
