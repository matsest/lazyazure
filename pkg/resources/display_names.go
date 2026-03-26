package resources

import (
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

//go:embed display_names.json
var displayNamesFS embed.FS

var displayNames map[string]string
var displayNamesLower map[string]string // case-insensitive lookup

func init() {
	if err := loadDisplayNames(); err != nil {
		// If we can't load the embedded file, start with empty maps
		// The fallback algorithm will handle all cases
		displayNames = make(map[string]string)
		displayNamesLower = make(map[string]string)
	}
}

func loadDisplayNames() error {
	data, err := displayNamesFS.ReadFile("display_names.json")
	if err != nil {
		return fmt.Errorf("failed to read display names file: %w", err)
	}

	var wrapper struct {
		Mappings map[string]string `json:"mappings"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return fmt.Errorf("failed to unmarshal display names: %w", err)
	}

	displayNames = wrapper.Mappings

	// Build lowercase map for case-insensitive lookups
	displayNamesLower = make(map[string]string, len(displayNames))
	for k, v := range displayNames {
		displayNamesLower[strings.ToLower(k)] = v
	}

	return nil
}

// GetResourceTypeDisplayName returns a human-readable name for an Azure resource type
// It first checks the core mapping, then falls back to an algorithm
func GetResourceTypeDisplayName(resourceType string) string {
	// Check core mapping first (exact match)
	if name, ok := displayNames[resourceType]; ok {
		return name
	}

	// Check case-insensitive mapping (Azure sometimes returns lowercase)
	if name, ok := displayNamesLower[strings.ToLower(resourceType)]; ok {
		return name
	}

	// Fallback to algorithm
	return generateDisplayName(resourceType)
}

// camelCaseRegex matches camelCase boundaries
var camelCaseRegex = regexp.MustCompile(`([a-z])([A-Z])`)

// pluralSuffixRegex matches common plural suffixes
var pluralSuffixRegex = regexp.MustCompile(`(s|es|ies)$`)

// knownAcronyms maps acronyms to their proper casing
var knownAcronyms = map[string]string{
	"ip":   "IP",
	"sql":  "SQL",
	"vm":   "VM",
	"nsg":  "NSG",
	"aks":  "AKS",
	"cdn":  "CDN",
	"dns":  "DNS",
	"vpn":  "VPN",
	"ddos": "DDoS",
	"nat":  "NAT",
}

func generateDisplayName(resourceType string) string {
	// Split into provider and resource name
	parts := strings.Split(resourceType, "/")
	if len(parts) < 2 {
		// Just return the last part if no provider
		return formatResourceName(resourceType)
	}

	provider := parts[0]        // e.g., "Microsoft.Compute"
	name := parts[len(parts)-1] // e.g., "virtualMachines"

	// Split camelCase into words
	words := splitCamelCase(name)

	if len(words) >= 2 {
		// Use resource name words
		return strings.Join(formatWords(words), " ")
	}

	// Single word: use provider name + resource name (singular)
	providerWords := extractProviderName(provider)
	resourceWord := strings.ToLower(singularize(words[0]))

	// Check if provider already ends with the resource name (case-insensitive)
	// e.g., "KeyVault" + "vault" -> just "Key Vault"
	providerLower := strings.ToLower(providerWords)
	if strings.HasSuffix(providerLower, " "+resourceWord) || providerLower == resourceWord {
		return providerWords
	}

	return providerWords + " " + strings.Title(resourceWord)
}

func splitCamelCase(s string) []string {
	// Insert space before capital letters
	spaced := camelCaseRegex.ReplaceAllString(s, "$1 $2")
	return strings.Fields(spaced)
}

func formatResourceName(name string) string {
	words := splitCamelCase(name)
	return strings.Join(formatWords(words), " ")
}

func formatWords(words []string) []string {
	result := make([]string, len(words))
	for i, word := range words {
		lower := strings.ToLower(word)

		// Check if it's a known acronym
		if acronym, ok := knownAcronyms[lower]; ok {
			result[i] = acronym
		} else if i == len(words)-1 {
			// Last word: try to singularize
			result[i] = strings.Title(singularize(word))
		} else {
			result[i] = strings.Title(lower)
		}
	}
	return result
}

func singularize(word string) string {
	lower := strings.ToLower(word)

	// Handle special cases - words ending in 's' that aren't plural
	switch lower {
	case "status", "access", "address", "class", "glass", "grass",
		"ingress", "egress", "yes", "is", "this", "was",
		"insights":
		return word
	}

	// Handle specific plural forms
	switch lower {
	case "services":
		return word[:len(word)-1] // Keep the 'e'
	case "addresses":
		return "address"
	case "machines":
		return word[:len(word)-1]
	case "tables":
		return word[:len(word)-1]
	case "queues":
		return word[:len(word)-1]
	case "parties":
		return word[:len(word)-3] + "y"
	case "buses":
		return word[:len(word)-2]
	case "lenses":
		return word[:len(word)-1] // "lenses" -> "lense" (but usually "lens")
	case "analyses":
		return word[:len(word)-2] // "analyses" -> "analysis" (irregular)
	}

	// Handle compound words ending in known plural forms
	// e.g., "IPAddresses", "DnsZones"
	if strings.HasSuffix(lower, "addresses") {
		return word[:len(word)-2] // Remove "es"
	}

	// Generic plural removal (only for simple 's' endings)
	if strings.HasSuffix(lower, "s") && !strings.HasSuffix(lower, "ss") &&
		!strings.HasSuffix(lower, "us") && !strings.HasSuffix(lower, "is") && len(word) > 1 {
		return word[:len(word)-1]
	}

	return word
}

func extractProviderName(provider string) string {
	// Remove "Microsoft." prefix
	name := strings.TrimPrefix(provider, "Microsoft.")

	// Split on camelCase and format
	words := splitCamelCase(name)
	formatted := formatWords(words)

	return strings.Join(formatted, " ")
}
