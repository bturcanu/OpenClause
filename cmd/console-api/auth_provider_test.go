package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bturcanu/OpenClause/pkg/console"
)

type fakeUserAuthenticator struct {
	u     *console.User
	roles []console.UserRole
	err   error
}

func (f *fakeUserAuthenticator) AuthenticateUser(ctx context.Context, email, password string) (*console.User, []console.UserRole, error) {
	return f.u, f.roles, f.err
}

type fakeAuthSessionIssuer struct {
	session *console.AuthSession
	err     error
	gotIn   console.AuthSessionCreateInput
}

func (f *fakeAuthSessionIssuer) CreateAuthSession(ctx context.Context, in console.AuthSessionCreateInput) (*console.AuthSession, error) {
	f.gotIn = in
	if f.err != nil {
		return nil, f.err
	}
	if f.session != nil {
		return f.session, nil
	}
	return &console.AuthSession{ID: "sess-123"}, nil
}

func testJWTConfig() console.JWTConfig {
	// 32+ bytes secret to satisfy token signing checks.
	secret := "0123456789abcdef0123456789abcdef"
	return console.JWTConfig{
		Secret:      secret,
		Issuer:      "openclause-console",
		ExpiryHours: 1,
	}
}

func Test_newAuthProvider_default(t *testing.T) {
	p, err := newAuthProvider("email_password", AuthProviderDeps{
		log:      slog.Default(),
		store:    &fakeUserAuthenticator{},
		sessions: &fakeAuthSessionIssuer{},
		jwtCfg:   testJWTConfig(),
	})
	if err != nil {
		t.Fatalf("newAuthProvider returned error: %v", err)
	}
	if p.Name() != "email_password" {
		t.Fatalf("expected provider name email_password, got %q", p.Name())
	}
}

func Test_newAuthProvider_unknown(t *testing.T) {
	_, err := newAuthProvider("weird", AuthProviderDeps{
		log:      slog.Default(),
		store:    &fakeUserAuthenticator{},
		sessions: &fakeAuthSessionIssuer{},
		jwtCfg:   testJWTConfig(),
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func Test_authProviderFromEnv_defaultEmailPassword(t *testing.T) {
	t.Setenv("CONSOLE_AUTH_PROVIDER", "")
	p, err := authProviderFromEnv(AuthProviderDeps{
		log:      slog.Default(),
		store:    &fakeUserAuthenticator{},
		sessions: &fakeAuthSessionIssuer{},
		jwtCfg:   testJWTConfig(),
	})
	if err != nil {
		t.Fatalf("authProviderFromEnv returned error: %v", err)
	}
	if p.Name() != "email_password" {
		t.Fatalf("expected email_password provider, got %q", p.Name())
	}
}

func Test_EmailPasswordAuthProvider_Login_rejectsNoTenantNonPlatformAdmin(t *testing.T) {
	roleNoTenant := (*string)(nil)
	u := &console.User{ID: "u1", Email: "a@b.c", Name: "A", Status: "active"}
	roles := []console.UserRole{{Role: "viewer", TenantID: roleNoTenant}}

	p := &EmailPasswordAuthProvider{
		log:      slog.Default(),
		store:    &fakeUserAuthenticator{u: u, roles: roles},
		sessions: &fakeAuthSessionIssuer{},
		jwtCfg:   testJWTConfig(),
	}

	_, err := p.Login(context.Background(), AuthLoginInput{Email: "x", Password: "y"})
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := err.(*AuthProviderError)
	if !ok {
		t.Fatalf("expected AuthProviderError, got %T", err)
	}
	if ae.Status != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", ae.Status)
	}
	if ae.Message != "user has no tenant assignment" {
		t.Fatalf("unexpected message: %q", ae.Message)
	}
}

func Test_EmailPasswordAuthProvider_Login_allowsPlatformAdminWithoutTenant(t *testing.T) {
	u := &console.User{ID: "u1", Email: "a@b.c", Name: "A", Status: "active"}
	roles := []console.UserRole{{Role: "platform_admin", TenantID: nil}}

	p := &EmailPasswordAuthProvider{
		log:      slog.Default(),
		store:    &fakeUserAuthenticator{u: u, roles: roles},
		sessions: &fakeAuthSessionIssuer{session: &console.AuthSession{ID: "sess-platform"}},
		jwtCfg:   testJWTConfig(),
	}

	res, err := p.Login(context.Background(), AuthLoginInput{Email: "x", Password: "y"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if res.Token == "" {
		t.Fatal("token missing")
	}

	claims, err := console.ValidateToken(testJWTConfig(), res.Token)
	if err != nil {
		t.Fatalf("token validation failed: %v", err)
	}
	if claims.Tenant != "" {
		t.Fatalf("expected empty tenant claim, got %q", claims.Tenant)
	}
	if claims.SID != "sess-platform" {
		t.Fatalf("expected sid claim sess-platform, got %q", claims.SID)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "platform_admin" {
		t.Fatalf("unexpected roles claim: %+v", claims.Roles)
	}
}

func Test_EmailPasswordAuthProvider_Login_platformAdminIgnoresTenantScopedRoles(t *testing.T) {
	u := &console.User{ID: "u1", Email: "a@b.c", Name: "A", Status: "active"}
	tenantID := "tenant-1"
	roles := []console.UserRole{
		{Role: "tenant_admin", TenantID: &tenantID},
		{Role: "platform_admin", TenantID: nil},
	}
	sessions := &fakeAuthSessionIssuer{session: &console.AuthSession{ID: "sess-platform"}}

	p := &EmailPasswordAuthProvider{
		log:      slog.Default(),
		store:    &fakeUserAuthenticator{u: u, roles: roles},
		sessions: sessions,
		jwtCfg:   testJWTConfig(),
	}

	res, err := p.Login(context.Background(), AuthLoginInput{Email: "x", Password: "y"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if sessions.gotIn.TenantID != "" {
		t.Fatalf("expected auth session tenant scope to stay empty for platform admin, got %q", sessions.gotIn.TenantID)
	}

	claims, err := console.ValidateToken(testJWTConfig(), res.Token)
	if err != nil {
		t.Fatalf("token validation failed: %v", err)
	}
	if claims.Tenant != "" {
		t.Fatalf("expected empty tenant claim for platform admin, got %q", claims.Tenant)
	}
}

func Test_EmailPasswordAuthProvider_Login_rejectsMultipleTenantAssignments(t *testing.T) {
	u := &console.User{ID: "u1", Email: "a@b.c", Name: "A", Status: "active"}
	tenantOne := "tenant-1"
	tenantTwo := "tenant-2"
	roles := []console.UserRole{
		{Role: "tenant_admin", TenantID: &tenantOne},
		{Role: "approver", TenantID: &tenantTwo},
	}

	p := &EmailPasswordAuthProvider{
		log:      slog.Default(),
		store:    &fakeUserAuthenticator{u: u, roles: roles},
		sessions: &fakeAuthSessionIssuer{},
		jwtCfg:   testJWTConfig(),
	}

	_, err := p.Login(context.Background(), AuthLoginInput{Email: "x", Password: "y"})
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := err.(*AuthProviderError)
	if !ok {
		t.Fatalf("expected AuthProviderError, got %T", err)
	}
	if ae.Status != http.StatusConflict {
		t.Fatalf("expected 409, got %d", ae.Status)
	}
	if ae.Message != "user has multiple tenant assignments" {
		t.Fatalf("unexpected message: %q", ae.Message)
	}
}

func Test_EmailPasswordAuthProvider_Login_singleTenantScopesSessionAndToken(t *testing.T) {
	u := &console.User{ID: "u1", Email: "a@b.c", Name: "A", Status: "active"}
	tenantID := "tenant-1"
	roles := []console.UserRole{{Role: "tenant_admin", TenantID: &tenantID}}
	sessions := &fakeAuthSessionIssuer{session: &console.AuthSession{ID: "sess-tenant"}}

	p := &EmailPasswordAuthProvider{
		log:      slog.Default(),
		store:    &fakeUserAuthenticator{u: u, roles: roles},
		sessions: sessions,
		jwtCfg:   testJWTConfig(),
	}

	res, err := p.Login(context.Background(), AuthLoginInput{Email: "x", Password: "y"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if sessions.gotIn.TenantID != tenantID {
		t.Fatalf("expected auth session tenant scope %q, got %q", tenantID, sessions.gotIn.TenantID)
	}

	claims, err := console.ValidateToken(testJWTConfig(), res.Token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Tenant != tenantID || claims.SID != "sess-tenant" {
		t.Fatalf("expected tenant-scoped token, got %+v", claims)
	}
}

func Test_handleLogin_dispatchesToAuthProvider(t *testing.T) {
	rec := &recordingProvider{
		res: &AuthLoginResponse{
			Token: "tok123",
			User: AuthUser{
				ID:    "u1",
				Email: "a@b.c",
				Name:  "A",
				Roles: []string{"platform_admin"},
			},
		},
	}

	api := &ConsoleAPI{
		authProvider: rec,
	}

	body, _ := json.Marshal(map[string]any{"email": "a@b.c", "password": "pw"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.handleLogin(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !rec.called {
		t.Fatal("expected provider Login to be called")
	}
	if rec.gotIn.Email != "a@b.c" {
		t.Fatalf("expected trimmed email, got %q", rec.gotIn.Email)
	}

	b, _ := io.ReadAll(resp.Body)
	var decoded AuthLoginResponse
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("failed to decode login response: %v (body=%s)", err, string(b))
	}
	if decoded.Token != "tok123" {
		t.Fatalf("unexpected token: %q", decoded.Token)
	}
}

func Test_handleLogin_trimsEmailBeforeDispatch(t *testing.T) {
	rec := &recordingProvider{
		res: &AuthLoginResponse{
			Token: "tok123",
			User: AuthUser{
				ID:    "u1",
				Email: "a@b.c",
				Name:  "A",
				Roles: []string{"platform_admin"},
			},
		},
	}

	api := &ConsoleAPI{
		authProvider: rec,
	}

	body, _ := json.Marshal(map[string]any{"email": "  a@b.c  ", "password": "pw"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.handleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if rec.gotIn.Email != "a@b.c" {
		t.Fatalf("expected trimmed email, got %q", rec.gotIn.Email)
	}
}

type recordingProvider struct {
	called bool
	gotIn  AuthLoginInput
	res    *AuthLoginResponse
	err    error
}

func (p *recordingProvider) Name() string { return "recording" }

func (p *recordingProvider) Login(ctx context.Context, in AuthLoginInput) (*AuthLoginResponse, error) {
	p.called = true
	p.gotIn = in
	return p.res, p.err
}

func (p *recordingProvider) Callback(ctx context.Context, _ map[string]any) (*AuthLoginResponse, error) {
	return nil, authProviderError(http.StatusNotImplemented, "callback not supported")
}
