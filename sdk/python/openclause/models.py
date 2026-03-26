"""Typed request and response models for the OpenClause API."""

from __future__ import annotations

from dataclasses import dataclass
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
    labels: Optional[Dict[str, str]] = None
    source_ip: str = ""
    trace_id: str = ""
    requested_at: str = ""
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
        if self.labels is not None:
            payload["labels"] = self.labels
        if self.source_ip:
            payload["source_ip"] = self.source_ip
        if self.trace_id:
            payload["trace_id"] = self.trace_id
        if self.requested_at:
            payload["requested_at"] = self.requested_at
        return payload

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> ToolCallRequest:
        return cls(
            tenant_id=data.get("tenant_id", ""),
            agent_id=data.get("agent_id", ""),
            tool=data.get("tool", ""),
            action=data.get("action", ""),
            idempotency_key=data.get("idempotency_key", ""),
            params=data.get("params"),
            resource=data.get("resource", ""),
            risk_score=data.get("risk_score"),
            risk_factors=data.get("risk_factors"),
            user_id=data.get("user_id", ""),
            session_id=data.get("session_id", ""),
            labels=data.get("labels"),
            source_ip=data.get("source_ip", ""),
            trace_id=data.get("trace_id", ""),
            requested_at=data.get("requested_at", ""),
            schema_version=data.get("schema_version", "1.0"),
        )


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
    request: Optional[ToolCallRequest] = None
    policy_result: Optional[Dict[str, Any]] = None
    execution_result: Optional[ExecutionResult] = None
    result: Optional[ExecutionResult] = None
    hash: str = ""
    prev_hash: str = ""
    received_at: str = ""
    tenant_id: str = ""
    agent_id: str = ""
    tool: str = ""
    action: str = ""
    reason: str = ""
    resource: str = ""
    risk_score: int = 0
    user_id: str = ""
    session_id: str = ""
    trace_id: str = ""
    labels: Optional[Dict[str, str]] = None
    source_ip: str = ""
    requested_at: str = ""

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> ToolCallEvent:
        request_data = data.get("request") or {}
        request = ToolCallRequest.from_dict(request_data) if request_data else None

        execution_data = data.get("execution_result") or data.get("result")
        execution_result = None
        if execution_data is not None:
            execution_result = ExecutionResult.from_dict(execution_data)

        if request is not None:
            tenant_id = request.tenant_id
            agent_id = request.agent_id
            tool = request.tool
            action = request.action
            resource = request.resource
            risk_score = request.risk_score or 0
            user_id = request.user_id
            session_id = request.session_id
            trace_id = request.trace_id
            labels = request.labels
            source_ip = request.source_ip
            requested_at = request.requested_at
        else:
            tenant_id = data.get("tenant_id", "")
            agent_id = data.get("agent_id", "")
            tool = data.get("tool", "")
            action = data.get("action", "")
            resource = data.get("resource", "")
            risk_score = data.get("risk_score", 0) or 0
            user_id = data.get("user_id", "")
            session_id = data.get("session_id", "")
            trace_id = data.get("trace_id", "")
            labels = data.get("labels")
            source_ip = data.get("source_ip", "")
            requested_at = data.get("requested_at", "")
        reason = data.get("reason", "")
        if not reason and isinstance(data.get("policy_result"), dict):
            policy_reason = data["policy_result"].get("reason")
            if isinstance(policy_reason, str):
                reason = policy_reason

        return cls(
            event_id=data.get("event_id", ""),
            decision=data.get("decision", ""),
            request=request,
            policy_result=data.get("policy_result"),
            execution_result=execution_result,
            result=execution_result,
            hash=data.get("hash", ""),
            prev_hash=data.get("prev_hash", ""),
            received_at=data.get("received_at", ""),
            tenant_id=tenant_id,
            agent_id=agent_id,
            tool=tool,
            action=action,
            reason=reason,
            resource=resource,
            risk_score=risk_score,
            user_id=user_id,
            session_id=session_id,
            trace_id=trace_id,
            labels=labels,
            source_ip=source_ip,
            requested_at=requested_at,
        )
