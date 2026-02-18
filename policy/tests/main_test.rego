# Tests for aag.main policy

package aag.main_test

import rego.v1

import data.aag.main

# ──────────────────────────────────────────────────────────────────────────────
# Test: allow low-risk read action
# ──────────────────────────────────────────────────────────────────────────────

test_allow_low_risk_read if {
	result := main.decision with input as {
		"toolcall": {
			"tenant_id": "tenant1",
			"agent_id": "agent-1",
			"tool": "jira",
			"action": "issue.list",
			"risk_score": 1,
			"idempotency_key": "key-1"
		},
		"environment": {
			"timestamp": "2025-01-01T00:00:00Z"
		}
	}
	result == "allow"
}

# ──────────────────────────────────────────────────────────────────────────────
# Test: approve high-risk action
# ──────────────────────────────────────────────────────────────────────────────

test_approve_high_risk if {
	result := main.decision with input as {
		"toolcall": {
			"tenant_id": "tenant1",
			"agent_id": "agent-1",
			"tool": "slack",
			"action": "msg.post",
			"risk_score": 8,
			"idempotency_key": "key-2"
		},
		"environment": {
			"timestamp": "2025-01-01T00:00:00Z"
		}
	}
	result == "approve"
}

# ──────────────────────────────────────────────────────────────────────────────
# Test: approve destructive action
# ──────────────────────────────────────────────────────────────────────────────

test_approve_destructive if {
	result := main.decision with input as {
		"toolcall": {
			"tenant_id": "tenant1",
			"agent_id": "agent-1",
			"tool": "jira",
			"action": "issue.delete",
			"risk_score": 3,
			"idempotency_key": "key-3"
		},
		"environment": {
			"timestamp": "2025-01-01T00:00:00Z"
		}
	}
	result == "approve"
}

# ──────────────────────────────────────────────────────────────────────────────
# Test: deny unknown action
# ──────────────────────────────────────────────────────────────────────────────

test_deny_unknown if {
	result := main.decision with input as {
		"toolcall": {
			"tenant_id": "tenant1",
			"agent_id": "agent-1",
			"tool": "unknown",
			"action": "do.something",
			"risk_score": 2,
			"idempotency_key": "key-4"
		},
		"environment": {
			"timestamp": "2025-01-01T00:00:00Z"
		}
	}
	result == "deny"
}

# ──────────────────────────────────────────────────────────────────────────────
# Test: allow moderate write action
# ──────────────────────────────────────────────────────────────────────────────

test_allow_write_moderate_risk if {
	result := main.decision with input as {
		"toolcall": {
			"tenant_id": "tenant1",
			"agent_id": "agent-1",
			"tool": "slack",
			"action": "msg.post",
			"risk_score": 4,
			"idempotency_key": "key-5"
		},
		"environment": {
			"timestamp": "2025-01-01T00:00:00Z"
		}
	}
	result == "allow"
}
