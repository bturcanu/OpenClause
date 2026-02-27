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
	tool_action in data.allowlist.destructive_actions
} else := "allow" if {
	tool_action := concat(".", [input.toolcall.tool, input.toolcall.action])
	tool_action in data.allowlist.read_actions
	input.toolcall.risk_score <= 3
} else := "allow" if {
	tool_action := concat(".", [input.toolcall.tool, input.toolcall.action])
	tool_action in data.allowlist.write_actions
	input.toolcall.risk_score < 7
}

reason := "high risk score requires approval" if {
	input.toolcall.risk_score >= 7
} else := "destructive action requires approval" if {
	tool_action := concat(".", [input.toolcall.tool, input.toolcall.action])
	tool_action in data.allowlist.destructive_actions
} else := "low-risk read action on allowlist" if {
	tool_action := concat(".", [input.toolcall.tool, input.toolcall.action])
	tool_action in data.allowlist.read_actions
	input.toolcall.risk_score <= 3
} else := "write action on allowlist with acceptable risk" if {
	tool_action := concat(".", [input.toolcall.tool, input.toolcall.action])
	tool_action in data.allowlist.write_actions
	input.toolcall.risk_score < 7
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
