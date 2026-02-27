# Tests for oc.main policy

package oc.main_test

import rego.v1

import data.oc.main

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

# ──────────────────────────────────────────────────────────────────────────────
# Test: CRITICAL conflict scenario — destructive action + high risk score
# Both conditions overlap; must produce a single decision without conflict.
# ──────────────────────────────────────────────────────────────────────────────

test_approve_destructive_high_risk if {
	result := main.decision with input as {
		"toolcall": {
			"tenant_id": "tenant1",
			"agent_id": "agent-1",
			"tool": "jira",
			"action": "issue.delete",
			"risk_score": 8,
			"idempotency_key": "key-6"
		},
		"environment": {
			"timestamp": "2025-01-01T00:00:00Z"
		}
	}
	result == "approve"
}

test_reason_destructive_high_risk if {
	result := main.reason with input as {
		"toolcall": {
			"tenant_id": "tenant1",
			"agent_id": "agent-1",
			"tool": "jira",
			"action": "issue.delete",
			"risk_score": 8,
			"idempotency_key": "key-6"
		},
		"environment": {
			"timestamp": "2025-01-01T00:00:00Z"
		}
	}
	result == "high risk score requires approval"
}

# ──────────────────────────────────────────────────────────────────────────────
# Boundary tests
# ──────────────────────────────────────────────────────────────────────────────

test_read_at_boundary_risk_3 if {
	result := main.decision with input as {
		"toolcall": {
			"tenant_id": "tenant1",
			"agent_id": "agent-1",
			"tool": "jira",
			"action": "issue.list",
			"risk_score": 3,
			"idempotency_key": "key-b1"
		},
		"environment": {}
	}
	result == "allow"
}

test_read_at_boundary_risk_4_denied if {
	result := main.decision with input as {
		"toolcall": {
			"tenant_id": "tenant1",
			"agent_id": "agent-1",
			"tool": "jira",
			"action": "issue.list",
			"risk_score": 4,
			"idempotency_key": "key-b2"
		},
		"environment": {}
	}
	result == "deny"
}

test_write_at_boundary_risk_6 if {
	result := main.decision with input as {
		"toolcall": {
			"tenant_id": "tenant1",
			"agent_id": "agent-1",
			"tool": "slack",
			"action": "msg.post",
			"risk_score": 6,
			"idempotency_key": "key-b3"
		},
		"environment": {}
	}
	result == "allow"
}

test_approve_at_boundary_risk_7 if {
	result := main.decision with input as {
		"toolcall": {
			"tenant_id": "tenant1",
			"agent_id": "agent-1",
			"tool": "slack",
			"action": "msg.post",
			"risk_score": 7,
			"idempotency_key": "key-b4"
		},
		"environment": {}
	}
	result == "approve"
}

test_deny_zero_risk_unknown_action if {
	result := main.decision with input as {
		"toolcall": {
			"tenant_id": "tenant1",
			"agent_id": "agent-1",
			"tool": "unknown",
			"action": "unknown",
			"risk_score": 0,
			"idempotency_key": "key-b5"
		},
		"environment": {}
	}
	result == "deny"
}

# ──────────────────────────────────────────────────────────────────────────────
# Requirements output test
# ──────────────────────────────────────────────────────────────────────────────

test_requirements_on_approve if {
	result := main.requirements with input as {
		"toolcall": {
			"tenant_id": "tenant1",
			"agent_id": "agent-1",
			"tool": "jira",
			"action": "issue.delete",
			"risk_score": 3,
			"idempotency_key": "key-r1"
		},
		"environment": {}
	}
	result.approval_scope == "single_use"
}

test_notify_routes_on_approve if {
	routes := main.notify with input as {
		"toolcall": {
			"tenant_id": "tenant1",
			"agent_id": "agent-1",
			"tool": "jira",
			"action": "issue.delete",
			"risk_score": 8,
			"idempotency_key": "key-n1"
		},
		"environment": {}
	}
	count(routes) >= 1
}
