package approvals

import (
	"context"
	"strings"
)

type ApproverLookup interface {
	FindUserByEmail(context.Context, string) (*ConsoleUserIdentity, error)
	FindUserBySlackUserID(context.Context, string) (*ConsoleUserIdentity, error)
	IsApproverUserForTenant(context.Context, string, string) (bool, error)
}

// allowlistSource controls which authorization sources are considered.
// Default is DB only.
type allowlistSource int

const (
	allowlistSourceDB allowlistSource = iota
	allowlistSourceEnv
	allowlistSourceBoth
)

type ApproverAuthorizer struct {
	lookup ApproverLookup

	emailByTenant map[string]map[string]struct{}
	slackByTenant map[string]map[string]struct{}

	src allowlistSource
}

// NewApproverAuthorizer gates authorization by DB (users+user_roles) and optionally by env allowlists.
// env allowlists are intended as a dev bootstrap fallback.
func NewApproverAuthorizer(lookup ApproverLookup, emailAllowlist, slackAllowlist string, source string) *ApproverAuthorizer {
	return &ApproverAuthorizer{
		lookup:        lookup,
		emailByTenant: parseTenantList(emailAllowlist),
		slackByTenant: parseTenantList(slackAllowlist),
		src:           parseAllowlistSource(source),
	}
}

func parseAllowlistSource(raw string) allowlistSource {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "env":
		return allowlistSourceEnv
	case "both":
		return allowlistSourceBoth
	case "", "db":
		return allowlistSourceDB
	default:
		// Fail closed to DB only on unknown config.
		return allowlistSourceDB
	}
}

func parseTenantList(raw string) map[string]map[string]struct{} {
	out := map[string]map[string]struct{}{}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 {
			continue
		}
		tenantID := strings.TrimSpace(parts[0])
		values := strings.Split(parts[1], "|")
		if tenantID == "" {
			continue
		}
		if _, ok := out[tenantID]; !ok {
			out[tenantID] = map[string]struct{}{}
		}
		for _, v := range values {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			out[tenantID][strings.ToLower(v)] = struct{}{}
		}
	}
	return out
}

func (a *ApproverAuthorizer) allowEmailEnv(tenantID, email string) bool {
	if email == "" {
		return false
	}
	allowed, ok := a.emailByTenant[tenantID]
	if !ok || len(allowed) == 0 {
		return false
	}
	_, ok = allowed[strings.ToLower(strings.TrimSpace(email))]
	return ok
}

func (a *ApproverAuthorizer) allowSlackEnv(tenantID, slackUserID string) bool {
	if slackUserID == "" {
		return false
	}
	allowed, ok := a.slackByTenant[tenantID]
	if !ok || len(allowed) == 0 {
		return false
	}
	_, ok = allowed[strings.ToLower(strings.TrimSpace(slackUserID))]
	return ok
}

// ResolveSlackApprover returns the console user email associated with the given Slack user id,
// but only if that user exists (slack_user_id) and is an approver for the tenant.
func (a *ApproverAuthorizer) ResolveSlackApprover(ctx context.Context, tenantID, slackUserID string) (email string, ok bool) {
	if tenantID == "" || slackUserID == "" || a.lookup == nil {
		return "", false
	}
	user, err := a.lookup.FindUserBySlackUserID(ctx, slackUserID)
	if err != nil || user == nil {
		return "", false
	}
	// Optional env allowlist check.
	envAllowed := a.allowSlackEnv(tenantID, slackUserID)

	dbAllowed, err := a.lookup.IsApproverUserForTenant(ctx, tenantID, user.ID)
	if err != nil {
		return "", false
	}

	switch a.src {
	case allowlistSourceDB:
		if !dbAllowed {
			return "", false
		}
	case allowlistSourceEnv:
		if !envAllowed {
			return "", false
		}
	case allowlistSourceBoth:
		if !dbAllowed && !envAllowed {
			return "", false
		}
	default:
		return "", false
	}

	return user.Email, true
}

// AllowEmail checks whether the given email identity is an approver for the tenant.
// DB is the primary source of truth; env allowlists are optional depending on allowlist source.
func (a *ApproverAuthorizer) AllowEmail(ctx context.Context, tenantID, email string) bool {
	if tenantID == "" || email == "" || a.lookup == nil {
		return false
	}
	user, err := a.lookup.FindUserByEmail(ctx, email)
	if err != nil || user == nil {
		return false
	}

	envAllowed := a.allowEmailEnv(tenantID, email)
	dbAllowed, err := a.lookup.IsApproverUserForTenant(ctx, tenantID, user.ID)
	if err != nil {
		return false
	}

	switch a.src {
	case allowlistSourceDB:
		return dbAllowed
	case allowlistSourceEnv:
		return envAllowed
	case allowlistSourceBoth:
		return dbAllowed || envAllowed
	default:
		return false
	}
}

func (a *ApproverAuthorizer) AllowSlack(ctx context.Context, tenantID, slackUserID string) bool {
	_, ok := a.ResolveSlackApprover(ctx, tenantID, slackUserID)
	return ok
}
