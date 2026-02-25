# OpenClause — Baseline Policy
# Package: oc.main
#
# Default deny. Allow low-risk reads. Require approval for high-risk / destructive.

package oc.main

import rego.v1

# ──────────────────────────────────────────────────────────────────────────────
# Default decision
# ──────────────────────────────────────────────────────────────────────────────

default decision := "deny"

default reason := "action not in allowlist"

# ──────────────────────────────────────────────────────────────────────────────
# Allow: low-risk read actions on the allowlist
# ──────────────────────────────────────────────────────────────────────────────

decision := "allow" if {
	tool_action := concat(".", [input.toolcall.tool, input.toolcall.action])
	tool_action in data.allowlist.read_actions
	input.toolcall.risk_score <= 3
}

reason := "low-risk read action on allowlist" if {
	tool_action := concat(".", [input.toolcall.tool, input.toolcall.action])
	tool_action in data.allowlist.read_actions
	input.toolcall.risk_score <= 3
}

# ──────────────────────────────────────────────────────────────────────────────
# Approve: high-risk score or destructive actions
# ──────────────────────────────────────────────────────────────────────────────

decision := "approve" if {
	input.toolcall.risk_score >= 7
}

reason := "high risk score requires approval" if {
	input.toolcall.risk_score >= 7
}

decision := "approve" if {
	tool_action := concat(".", [input.toolcall.tool, input.toolcall.action])
	tool_action in data.allowlist.destructive_actions
}

reason := "destructive action requires approval" if {
	tool_action := concat(".", [input.toolcall.tool, input.toolcall.action])
	tool_action in data.allowlist.destructive_actions
}

# ──────────────────────────────────────────────────────────────────────────────
# Allow: known write actions with moderate risk
# ──────────────────────────────────────────────────────────────────────────────

decision := "allow" if {
	tool_action := concat(".", [input.toolcall.tool, input.toolcall.action])
	tool_action in data.allowlist.write_actions
	input.toolcall.risk_score < 7
}

reason := "write action on allowlist with acceptable risk" if {
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
