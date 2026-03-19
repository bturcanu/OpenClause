package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/bturcanu/OpenClause/pkg/console"
)

// AuthLoginInput represents the credentials submitted to the console login endpoint.
// OIDC will map its user claims into the same semantic inputs/outputs.
type AuthLoginInput struct {
	Email    string
	Password string
}

type AuthUser struct {
	ID    string   `json:"id"`
	Email string   `json:"email"`
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

type AuthLoginResponse struct {
	Token string   `json:"token"`
	User  AuthUser `json:"user"`
}

// AuthProvider is the seam that allows swapping authentication mechanisms
// (e.g. email/password today, OIDC tomorrow) without rewriting login handlers.
type AuthProvider interface {
	Name() string

	Login(ctx context.Context, in AuthLoginInput) (*AuthLoginResponse, error)

	// These methods are part of the contract for OIDC-like providers.
	// The email/password provider explicitly returns "not supported".
	Callback(ctx context.Context, _ map[string]any) (*AuthLoginResponse, error)
}

type AuthProviderError struct {
	Status  int
	Message string
}

func (e *AuthProviderError) Error() string {
	return fmt.Sprintf("[%d] %s", e.Status, e.Message)
}

func authProviderError(status int, message string) error {
	return &AuthProviderError{Status: status, Message: message}
}

type UserAuthenticator interface {
	AuthenticateUser(ctx context.Context, email, password string) (*console.User, []console.UserRole, error)
}

type EmailPasswordAuthProvider struct {
	log      *slog.Logger
	store    UserAuthenticator
	jwtCfg   console.JWTConfig
	issuerID string
}

func (p *EmailPasswordAuthProvider) Name() string { return "email_password" }

func (p *EmailPasswordAuthProvider) Callback(ctx context.Context, _ map[string]any) (*AuthLoginResponse, error) {
	// Not used by the existing /auth/login flow.
	return nil, authProviderError(http.StatusNotImplemented, "callback not supported")
}

func (p *EmailPasswordAuthProvider) Login(ctx context.Context, in AuthLoginInput) (*AuthLoginResponse, error) {
	user, roles, err := p.store.AuthenticateUser(ctx, in.Email, in.Password)
	if err != nil {
		return nil, authProviderError(http.StatusUnauthorized, "invalid credentials")
	}

	roleNames := make([]string, len(roles))
	var scopedTenant string
	for i, role := range roles {
		roleNames[i] = role.Role
		if role.TenantID != nil {
			scopedTenant = *role.TenantID
		}
	}

	// Preserve existing behavior:
	// - Reject non-platform_admin users without tenant assignment.
	if scopedTenant == "" && !containsRole(roleNames, "platform_admin") {
		return nil, authProviderError(http.StatusForbidden, "user has no tenant assignment")
	}

	token, err := console.GenerateToken(p.jwtCfg, console.JWTClaims{
		Sub:    user.ID,
		Email:  user.Email,
		Name:   user.Name,
		Roles:  roleNames,
		Tenant: scopedTenant,
	})
	if err != nil {
		if p.log != nil {
			p.log.Error("generate token failed", "error", err)
		}
		return nil, authProviderError(http.StatusInternalServerError, "failed to generate token")
	}

	return &AuthLoginResponse{
		Token: token,
		User: AuthUser{
			ID:    user.ID,
			Email: user.Email,
			Name:  user.Name,
			Roles: roleNames,
		},
	}, nil
}

type AuthProviderDeps struct {
	log    *slog.Logger
	store  UserAuthenticator
	jwtCfg console.JWTConfig
}

func authProviderFromEnv(deps AuthProviderDeps) (AuthProvider, error) {
	// Default provider preserves existing behavior.
	name := strings.ToLower(strings.TrimSpace(os.Getenv("CONSOLE_AUTH_PROVIDER")))
	if name == "" {
		name = "email_password"
	}
	return newAuthProvider(name, deps)
}

func newAuthProvider(name string, deps AuthProviderDeps) (AuthProvider, error) {
	switch name {
	case "email_password", "password", "local":
		return &EmailPasswordAuthProvider{
			log:    deps.log,
			store:  deps.store,
			jwtCfg: deps.jwtCfg,
		}, nil
	default:
		return nil, fmt.Errorf("unknown auth provider %q", name)
	}
}

var errAuthProviderNotSupported = errors.New("auth provider callback not supported")

