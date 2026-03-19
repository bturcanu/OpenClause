package approvals

import (
	"context"
	"testing"
)

type fakeLookup struct {
	usersByEmail map[string]*ConsoleUserIdentity
	usersBySlack map[string]*ConsoleUserIdentity

	approverFor map[string]map[string]bool // tenant -> userID -> allowed
}

func (f *fakeLookup) FindUserByEmail(ctx context.Context, email string) (*ConsoleUserIdentity, error) {
	return f.usersByEmail[email], nil
}

func (f *fakeLookup) FindUserBySlackUserID(ctx context.Context, slackUserID string) (*ConsoleUserIdentity, error) {
	return f.usersBySlack[slackUserID], nil
}

func (f *fakeLookup) IsApproverUserForTenant(ctx context.Context, tenantID, userID string) (bool, error) {
	if f.approverFor == nil {
		return false, nil
	}
	if f.approverFor[tenantID] == nil {
		return false, nil
	}
	return f.approverFor[tenantID][userID], nil
}

func TestApproverAuthorizer_DBPrimary_DefaultDenyTenantScoped(t *testing.T) {
	lookup := &fakeLookup{
		usersByEmail: map[string]*ConsoleUserIdentity{
			"alice@acme.dev": {ID: "u1", Email: "alice@acme.dev"},
		},
		usersBySlack: map[string]*ConsoleUserIdentity{},
		approverFor: map[string]map[string]bool{
			"tenantA": {"u1": true},
			"tenantB": {"u1": false},
		},
	}

	a := NewApproverAuthorizer(lookup, "", "", "db")

	if !a.AllowEmail(context.Background(), "tenantA", "alice@acme.dev") {
		t.Fatalf("expected allow for tenantA")
	}
	if a.AllowEmail(context.Background(), "tenantB", "alice@acme.dev") {
		t.Fatalf("expected deny for tenantB")
	}
}

func TestApproverAuthorizer_EnvFallback_BothSource(t *testing.T) {
	lookup := &fakeLookup{
		usersByEmail: map[string]*ConsoleUserIdentity{
			"bob@acme.dev": {ID: "u2", Email: "bob@acme.dev"},
		},
		usersBySlack: map[string]*ConsoleUserIdentity{},
		approverFor: map[string]map[string]bool{
			"tenantA": {"u2": false},
		},
	}

	// env allowlists should allow if db doesn't when source=both.
	a := NewApproverAuthorizer(lookup, "tenantA:bob@acme.dev", "", "both")

	if !a.AllowEmail(context.Background(), "tenantA", "bob@acme.dev") {
		t.Fatalf("expected allow from env fallback when source=both")
	}
	if a.AllowEmail(context.Background(), "tenantA", "charlie@acme.dev") {
		t.Fatalf("expected deny for unknown email")
	}
}

func TestApproverAuthorizer_RemovalTakesEffectImmediately(t *testing.T) {
	lookup := &fakeLookup{
		usersByEmail: map[string]*ConsoleUserIdentity{
			"carol@acme.dev": {ID: "u3", Email: "carol@acme.dev"},
		},
		usersBySlack: map[string]*ConsoleUserIdentity{},
		approverFor: map[string]map[string]bool{
			"tenantA": {"u3": true},
		},
	}

	a := NewApproverAuthorizer(lookup, "", "", "db")

	if !a.AllowEmail(context.Background(), "tenantA", "carol@acme.dev") {
		t.Fatalf("expected allow initially")
	}

	// Simulate role removal without restart.
	lookup.approverFor["tenantA"]["u3"] = false

	if a.AllowEmail(context.Background(), "tenantA", "carol@acme.dev") {
		t.Fatalf("expected deny after removal")
	}
}

func TestApproverAuthorizer_SlackResolution_EnforcesTenantApproverRole(t *testing.T) {
	lookup := &fakeLookup{
		usersByEmail: map[string]*ConsoleUserIdentity{},
		usersBySlack: map[string]*ConsoleUserIdentity{
			"U12345678": {ID: "u4", Email: "dana@acme.dev"},
		},
		approverFor: map[string]map[string]bool{
			"tenantA": {"u4": true},
			"tenantB": {"u4": false},
		},
	}

	a := NewApproverAuthorizer(lookup, "", "", "db")

	email, ok := a.ResolveSlackApprover(context.Background(), "tenantA", "U12345678")
	if !ok || email != "dana@acme.dev" {
		t.Fatalf("expected slack tenantA to resolve and allow")
	}

	_, ok = a.ResolveSlackApprover(context.Background(), "tenantB", "U12345678")
	if ok {
		t.Fatalf("expected deny for slack when tenant role removed")
	}
}
