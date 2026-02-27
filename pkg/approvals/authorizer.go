package approvals

import "strings"

type ApproverAuthorizer struct {
	emailByTenant map[string]map[string]struct{}
	slackByTenant map[string]map[string]struct{}
}

func NewApproverAuthorizer(emailAllowlist, slackAllowlist string) *ApproverAuthorizer {
	return &ApproverAuthorizer{
		emailByTenant: parseTenantList(emailAllowlist),
		slackByTenant: parseTenantList(slackAllowlist),
	}
}

func (a *ApproverAuthorizer) AllowEmail(tenantID, email string) bool {
	if email == "" {
		return false
	}
	allowed, ok := a.emailByTenant[tenantID]
	if !ok || len(allowed) == 0 {
		return true
	}
	_, ok = allowed[strings.ToLower(strings.TrimSpace(email))]
	return ok
}

func (a *ApproverAuthorizer) AllowSlack(tenantID, userID string) bool {
	if userID == "" {
		return false
	}
	allowed, ok := a.slackByTenant[tenantID]
	if !ok || len(allowed) == 0 {
		return true
	}
	_, ok = allowed[strings.ToLower(strings.TrimSpace(userID))]
	return ok
}

func parseTenantList(raw string) map[string]map[string]struct{} {
	out := map[string]map[string]struct{}{}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
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
