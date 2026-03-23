package domain

// User represents the authenticated Azure user
type User struct {
	DisplayName       string `json:"displayName"`
	UserPrincipalName string `json:"userPrincipalName"`
	Type              string `json:"type"` // "user" or "serviceprincipal"
	TenantID          string `json:"tenantId"`
}

// IsAuthenticated returns true if the user has valid authentication
type IsAuthenticated bool
