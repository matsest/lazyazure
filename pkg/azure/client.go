package azure

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/golang-jwt/jwt/v5"
	"github.com/matsest/lazyazure/pkg/domain"
	"github.com/matsest/lazyazure/pkg/utils"
)

// Client wraps Azure SDK clients and provides high-level operations
type Client struct {
	credential         azcore.TokenCredential
	subscriptionClient *SubscriptionsClient
}

// InitSubscriptionsClient initializes the subscription client after client creation
func (c *Client) InitSubscriptionsClient() (*SubscriptionsClient, error) {
	return NewSubscriptionsClient(c)
}

// NewClient creates a new Azure client using DefaultAzureCredential
func NewClient() (*Client, error) {
	// Use DefaultAzureCredential which automatically:
	// 1. Checks environment variables
	// 2. Checks for managed identity
	// 3. Falls back to Azure CLI credentials (what we want for this app)
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure credential: %w", err)
	}

	return &Client{
		credential: credential,
	}, nil
}

// azureTokenClaims represents the claims we expect in an Azure access token
type azureTokenClaims struct {
	TenantID          string `json:"tid"`
	ObjectID          string `json:"oid"`
	UserPrincipalName string `json:"upn"`
	PreferredUsername string `json:"preferred_username"`
	AppID             string `json:"appid"` // v1.0 tokens
	Azp               string `json:"azp"`   // v2.0 tokens (replaces appid)
	Name              string `json:"name"`
	IdentityProvider  string `json:"idp"`
	Audience          string `json:"aud"`
	Issuer            string `json:"iss"`
}

// GetUserInfo retrieves information about the currently authenticated user
// by parsing the access token JWT. This works with any authentication method
// supported by DefaultAzureCredential.
func (c *Client) GetUserInfo(ctx context.Context) (*domain.User, error) {
	// Get access token from the credential
	tokenResponse, err := c.credential.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	// Parse the JWT token to extract claims
	claims, err := parseAzureToken(tokenResponse.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to parse access token: %w", err)
	}

	// Debug logging to see what claims are present
	utils.Log("GetUserInfo: Token claims - tid=%s, oid=%s, upn=%s, preferred_username=%s, appid=%s, azp=%s, name=%s",
		claims.TenantID, claims.ObjectID, claims.UserPrincipalName, claims.PreferredUsername,
		claims.AppID, claims.Azp, claims.Name)

	user := &domain.User{
		TenantID: claims.TenantID,
	}

	// Determine if this is a service principal or user
	// Users have upn, preferred_username, or name claims (the name claim indicates a user identity)
	// Service principals don't have user identity claims but have appid/azp
	hasUserIdentity := claims.UserPrincipalName != "" || claims.PreferredUsername != "" || claims.Name != ""

	utils.Log("GetUserInfo: hasUserIdentity=%v", hasUserIdentity)

	if !hasUserIdentity && (claims.AppID != "" || claims.Azp != "") {
		// Service Principal acting on its own (app-only token)
		user.Type = "serviceprincipal"
		// Use appid if available, otherwise azp
		appId := claims.AppID
		if appId == "" {
			appId = claims.Azp
		}
		user.UserPrincipalName = appId
		user.DisplayName = appId
		utils.Log("GetUserInfo: Detected as Service Principal, appId=%s", appId)
	} else {
		// Regular user (delegated token)
		user.Type = "user"

		// Use UPN if available, otherwise preferred_username, otherwise object ID
		if claims.UserPrincipalName != "" {
			user.UserPrincipalName = claims.UserPrincipalName
		} else if claims.PreferredUsername != "" {
			user.UserPrincipalName = claims.PreferredUsername
		} else if claims.ObjectID != "" {
			// Fallback to object ID if no username available
			user.UserPrincipalName = claims.ObjectID
		}

		// Use name claim for display name if available, otherwise fall back to UPN
		if claims.Name != "" {
			user.DisplayName = claims.Name
		} else {
			user.DisplayName = user.UserPrincipalName
		}
		utils.Log("GetUserInfo: Detected as User, UPN=%s, DisplayName=%s", user.UserPrincipalName, user.DisplayName)
	}

	return user, nil
}

// parseAzureToken parses an Azure access token JWT and extracts the claims.
// The token is not verified as it comes from the trusted Azure SDK.
func parseAzureToken(tokenString string) (*azureTokenClaims, error) {
	// Try parsing with jwt library first
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err == nil {
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			return mapClaimsToAzureClaims(claims), nil
		}
	}

	// Fallback: manual JWT parsing if jwt library fails
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}

	// Decode the payload (second part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var claims azureTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	return &claims, nil
}

// mapClaimsToAzureClaims converts jwt.MapClaims to azureTokenClaims
func mapClaimsToAzureClaims(mapClaims jwt.MapClaims) *azureTokenClaims {
	claims := &azureTokenClaims{}

	if v, ok := mapClaims["tid"].(string); ok {
		claims.TenantID = v
	}
	if v, ok := mapClaims["oid"].(string); ok {
		claims.ObjectID = v
	}
	if v, ok := mapClaims["upn"].(string); ok {
		claims.UserPrincipalName = v
	}
	if v, ok := mapClaims["preferred_username"].(string); ok {
		claims.PreferredUsername = v
	}
	if v, ok := mapClaims["appid"].(string); ok {
		claims.AppID = v
	}
	if v, ok := mapClaims["azp"].(string); ok {
		claims.Azp = v
	}
	if v, ok := mapClaims["name"].(string); ok {
		claims.Name = v
	}
	if v, ok := mapClaims["idp"].(string); ok {
		claims.IdentityProvider = v
	}
	if v, ok := mapClaims["aud"].(string); ok {
		claims.Audience = v
	}
	if v, ok := mapClaims["iss"].(string); ok {
		claims.Issuer = v
	}

	return claims
}

// VerifyAuthentication checks if the client can authenticate
func (c *Client) VerifyAuthentication(ctx context.Context) error {
	_, err := c.credential.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	return err
}

// Credential returns the underlying token credential
func (c *Client) Credential() azcore.TokenCredential {
	return c.credential
}
