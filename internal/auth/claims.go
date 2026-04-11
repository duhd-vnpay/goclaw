// internal/auth/claims.go
package auth

// KeycloakClaims contains the extracted claims from a validated Keycloak JWT.
type KeycloakClaims struct {
	KeycloakID   string   // sub claim = Keycloak user UUID
	Email        string   // email claim
	Name         string   // name claim (display name)
	RealmRoles   []string // realm_roles custom claim
	Groups       []string // groups custom claim
	AuthProvider string   // identity provider (google, microsoft, etc.)
}
