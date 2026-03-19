# OpenClause — Baseline Policy
# Package: oc.main
#
# Default deny. Allow low-risk reads. Require approval for high-risk / destructive.
# Rules use `else` chaining to avoid conflicting complete-rule outputs.

package oc.main

import rego.v1

# ──────────────────────────────────────────────────────────────────────────────
# Default decision
# ──────────────────────────────────────────────────────────────────────────────

default decision := "deny"

default reason := "action not in allowlist"

# ──────────────────────────────────────────────────────────────────────────────
# Priority 1: High-risk score → approve (checked first regardless of lists)
# ──────────────────────────────────────────────────────────────────────────────

decision := "approve" if {
	input.toolcall.risk_score >= 7
} else := "approve" if {
	tool_action := concat(".", [input.toolcall.tool, input.toolcall.action])
	destructive_requires_approval
	tool_action in effective_destructive_actions
} else := "allow" if {
	tool_action := concat(".", [input.toolcall.tool, input.toolcall.action])
	tool_action in effective_read_actions
	threshold := effective_max_risk_auto_approve
	input.toolcall.risk_score <= threshold
} else := "allow" if {
	tool_action := concat(".", [input.toolcall.tool, input.toolcall.action])
	tool_action in effective_write_actions
	threshold := effective_max_risk_auto_approve
	input.toolcall.risk_score <= threshold
}

reason := "high risk score requires approval" if {
	input.toolcall.risk_score >= 7
} else := "destructive action requires approval" if {
	tool_action := concat(".", [input.toolcall.tool, input.toolcall.action])
	destructive_requires_approval
	tool_action in effective_destructive_actions
} else := "read action on allowlist within tenant threshold" if {
	tool_action := concat(".", [input.toolcall.tool, input.toolcall.action])
	tool_action in effective_read_actions
	threshold := effective_max_risk_auto_approve
	input.toolcall.risk_score <= threshold
} else := "write action on allowlist within tenant threshold" if {
	tool_action := concat(".", [input.toolcall.tool, input.toolcall.action])
	tool_action in effective_write_actions
	threshold := effective_max_risk_auto_approve
	input.toolcall.risk_score <= threshold
}

effective_max_risk_auto_approve := threshold if {
	raw := object.get(object.get(input.environment, "tenant_config", {}), "max_risk_auto_approve", "")
	raw != ""
	threshold := to_number(raw)
} else := threshold if {
	threshold := object.get(object.get(data.tenants, input.toolcall.tenant_id, {}), "max_risk_auto_approve", 7)
}

destructive_requires_approval if {
	raw := lower(object.get(object.get(input.environment, "tenant_config", {}), "require_destructive_approval", ""))
	raw == "true"
} else if {
	raw := lower(object.get(object.get(input.environment, "tenant_config", {}), "require_destructive_approval", ""))
	raw == ""
}

effective_read_actions := actions if {
	raw := object.get(object.get(input.environment, "tenant_config", {}), "read_actions_csv", "")
	raw != ""
	parts := split(raw, ",")
	actions := [trim(p) |
		p := parts[_]
		trim(p) != ""
	]
} else := actions if {
	actions := data.allowlist.read_actions
}

effective_write_actions := actions if {
	raw := object.get(object.get(input.environment, "tenant_config", {}), "write_actions_csv", "")
	raw != ""
	parts := split(raw, ",")
	actions := [trim(p) |
		p := parts[_]
		trim(p) != ""
	]
} else := actions if {
	actions := data.allowlist.write_actions
}

effective_destructive_actions := actions if {
	raw := object.get(object.get(input.environment, "tenant_config", {}), "destructive_actions_csv", "")
	raw != ""
	parts := split(raw, ",")
	actions := [trim(p) |
		p := parts[_]
		trim(p) != ""
	]
} else := actions if {
	actions := data.allowlist.destructive_actions
}

trim(s) := out if {
	out := trim_space(s)
}

# ──────────────────────────────────────────────────────────────────────────────
# Output: requirements for approve decisions
# ──────────────────────────────────────────────────────────────────────────────

requirements := {"approval_scope": "single_use"} if {
	decision == "approve"
}

default notify := []

notify := routes if {
	decision == "approve"
	routes := object.get(data.tenants[input.toolcall.tenant_id], "notify", [])
}

default approver_group := ""

approver_group := grp if {
	decision == "approve"
	grp := object.get(data.tenants[input.toolcall.tenant_id], "approver_group", "")
}
