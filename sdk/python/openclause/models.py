"""Typed request and response models for the OpenClause API."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional


@dataclass
class ToolCallRequest:
    """Represents a tool-call submission to the OpenClause gateway."""

    tenant_id: str
    agent_id: str
    tool: str
    action: str
    idempotency_key: str
    params: Optional[Dict[str, Any]] = None
    resource: str = ""
    # Optional so that callers can explicitly send `risk_score=0` while still
    # allowing omission for a minimal payload (server default is 0).
    risk_score: Optional[int] = None
    risk_factors: Optional[List[str]] = None
    user_id: str = ""
    session_id: str = ""
    trace_id: str = ""
    schema_version: str = "1.0"

    def to_dict(self) -> Dict[str, Any]:
        payload: Dict[str, Any] = {
            "tenant_id": self.tenant_id,
            "agent_id": self.agent_id,
            "tool": self.tool,
            "action": self.action,
            "idempotency_key": self.idempotency_key,
            "schema_version": self.schema_version,
        }
        if self.params is not None:
            payload["params"] = self.params
        if self.resource:
            payload["resource"] = self.resource
        if self.risk_score is not None:
            payload["risk_score"] = self.risk_score
        if self.risk_factors:
            payload["risk_factors"] = self.risk_factors
        if self.user_id:
            payload["user_id"] = self.user_id
        if self.session_id:
            payload["session_id"] = self.session_id
        if self.trace_id:
            payload["trace_id"] = self.trace_id
        return payload


@dataclass
class ExecutionResult:
    """Result returned after a tool call is executed."""

    status: str
    output_json: Optional[Dict[str, Any]] = None
    error: str = ""
    duration_ms: int = 0

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> ExecutionResult:
        return cls(
            status=data.get("status", ""),
            output_json=data.get("output_json"),
            error=data.get("error", ""),
            duration_ms=data.get("duration_ms", 0),
        )


@dataclass
class ToolCallResponse:
    """Response from the OpenClause gateway for a tool-call operation."""

    event_id: str
    decision: str
    reason: str = ""
    approval_url: str = ""
    result: Optional[ExecutionResult] = None

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> ToolCallResponse:
        result = None
        if data.get("result") is not None:
            result = ExecutionResult.from_dict(data["result"])
        return cls(
            event_id=data.get("event_id", ""),
            decision=data.get("decision", ""),
            reason=data.get("reason", ""),
            approval_url=data.get("approval_url", ""),
            result=result,
        )


@dataclass
class ToolCallEvent:
    """Full event record returned by GET /v1/toolcalls/{event_id}."""

    event_id: str
    decision: str
    tenant_id: str = ""
    agent_id: str = ""
    tool: str = ""
    action: str = ""
    reason: str = ""
    approval_url: str = ""
    result: Optional[ExecutionResult] = None

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> ToolCallEvent:
        result = None
        if data.get("result") is not None:
            result = ExecutionResult.from_dict(data["result"])
        return cls(
            event_id=data.get("event_id", ""),
            decision=data.get("decision", ""),
            tenant_id=data.get("tenant_id", ""),
            agent_id=data.get("agent_id", ""),
            tool=data.get("tool", ""),
            action=data.get("action", ""),
            reason=data.get("reason", ""),
            approval_url=data.get("approval_url", ""),
            result=result,
        )
