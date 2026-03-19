"""Unit tests for the OpenClause Python SDK."""

from __future__ import annotations

import json
import unittest
from unittest.mock import MagicMock, patch

from openclause import (
    OpenClauseClient,
    ToolCallRequest,
    ToolCallResponse,
    ExecutionResult,
)
from openclause.exceptions import APIError, AuthenticationError, ValidationError


class TestToolCallRequestSerialization(unittest.TestCase):
    """ToolCallRequest.to_dict() produces the correct payload."""

    def test_minimal_request(self) -> None:
        req = ToolCallRequest(
            tenant_id="t1",
            agent_id="a1",
            tool="slack",
            action="msg.post",
            idempotency_key="key-1",
        )
        d = req.to_dict()
        self.assertEqual(d["tenant_id"], "t1")
        self.assertEqual(d["agent_id"], "a1")
        self.assertEqual(d["tool"], "slack")
        self.assertEqual(d["action"], "msg.post")
        self.assertEqual(d["idempotency_key"], "key-1")
        self.assertEqual(d["schema_version"], "1.0")
        self.assertNotIn("params", d)
        self.assertNotIn("resource", d)
        self.assertNotIn("risk_score", d)

    def test_full_request(self) -> None:
        req = ToolCallRequest(
            tenant_id="t1",
            agent_id="a1",
            tool="github",
            action="pr.merge",
            idempotency_key="key-2",
            params={"pr": 42},
            resource="repo/main",
            risk_score=8,
            risk_factors=["high-impact"],
            user_id="u1",
            session_id="s1",
            trace_id="trace-abc",
        )
        d = req.to_dict()
        self.assertEqual(d["params"], {"pr": 42})
        self.assertEqual(d["resource"], "repo/main")
        self.assertEqual(d["risk_score"], 8)
        self.assertEqual(d["risk_factors"], ["high-impact"])
        self.assertEqual(d["user_id"], "u1")
        self.assertEqual(d["session_id"], "s1")
        self.assertEqual(d["trace_id"], "trace-abc")

    def test_risk_score_zero_included_when_set(self) -> None:
        req = ToolCallRequest(
            tenant_id="t1",
            agent_id="a1",
            tool="slack",
            action="msg.post",
            idempotency_key="key-0",
            risk_score=0,
        )
        d = req.to_dict()
        self.assertEqual(d["risk_score"], 0)


class TestRiskScoreValidation(unittest.TestCase):
    def test_rejects_non_int_risk_score(self) -> None:
        client = OpenClauseClient(base_url="http://localhost:8080", api_key="sk-test")
        req = ToolCallRequest(  # type: ignore[arg-type]
            tenant_id="t1",
            agent_id="a1",
            tool="slack",
            action="msg.post",
            idempotency_key="key-1",
            risk_score=1.5,
        )
        with self.assertRaises(ValidationError):
            client.submit_tool_call(req)

    def test_rejects_out_of_range_risk_score(self) -> None:
        client = OpenClauseClient(base_url="http://localhost:8080", api_key="sk-test")
        req = ToolCallRequest(
            tenant_id="t1",
            agent_id="a1",
            tool="slack",
            action="msg.post",
            idempotency_key="key-2",
            risk_score=11,
        )
        with self.assertRaises(ValidationError):
            client.submit_tool_call(req)

    @patch("openclause.client.requests.Session")
    def test_includes_zero_risk_score_in_request_body(self, mock_session_cls: MagicMock) -> None:
        client = OpenClauseClient(base_url="http://localhost:8080", api_key="sk-test")
        client._post = MagicMock(  # type: ignore[method-assign]
            return_value={
                "event_id": "evt-0",
                "decision": "allow",
                "reason": "ok",
            }
        )
        req = ToolCallRequest(
            tenant_id="t1",
            agent_id="a1",
            tool="slack",
            action="msg.post",
            idempotency_key="key-0",
            risk_score=0,
        )
        _ = client.submit_tool_call(req)
        _, body = client._post.call_args[0][0:2]  # path, body
        self.assertEqual(body["risk_score"], 0)


class TestToolCallResponseDeserialization(unittest.TestCase):
    """ToolCallResponse.from_dict() parses JSON payloads correctly."""

    def test_simple_allow(self) -> None:
        data = {
            "event_id": "evt-1",
            "decision": "allow",
            "reason": "low risk",
        }
        resp = ToolCallResponse.from_dict(data)
        self.assertEqual(resp.event_id, "evt-1")
        self.assertEqual(resp.decision, "allow")
        self.assertEqual(resp.reason, "low risk")
        self.assertIsNone(resp.result)

    def test_with_execution_result(self) -> None:
        data = {
            "event_id": "evt-2",
            "decision": "allow",
            "result": {
                "status": "success",
                "output_json": {"ok": True},
                "duration_ms": 120,
            },
        }
        resp = ToolCallResponse.from_dict(data)
        self.assertIsNotNone(resp.result)
        self.assertEqual(resp.result.status, "success")
        self.assertEqual(resp.result.output_json, {"ok": True})
        self.assertEqual(resp.result.duration_ms, 120)
        self.assertEqual(resp.result.error, "")

    def test_missing_optional_fields(self) -> None:
        data = {"event_id": "evt-3", "decision": "deny"}
        resp = ToolCallResponse.from_dict(data)
        self.assertEqual(resp.reason, "")
        self.assertEqual(resp.approval_url, "")


class TestIdempotencyKey(unittest.TestCase):
    """generate_idempotency_key() produces unique values."""

    def test_uniqueness(self) -> None:
        keys = {OpenClauseClient.generate_idempotency_key() for _ in range(1000)}
        self.assertEqual(len(keys), 1000)

    def test_format(self) -> None:
        key = OpenClauseClient.generate_idempotency_key()
        parts = key.split("-")
        self.assertEqual(len(parts), 5, "UUID4 should have 5 dash-separated groups")


class TestBodySizeLimit(unittest.TestCase):
    """Requests exceeding 1 MB are rejected before sending."""

    @patch("openclause.client.requests.Session")
    def test_oversized_body_raises(self, mock_session_cls: MagicMock) -> None:
        client = OpenClauseClient(
            base_url="http://localhost:8080",
            api_key="sk-test",
        )
        huge_params = {"data": "x" * (2 * 1024 * 1024)}
        req = ToolCallRequest(
            tenant_id="t1",
            agent_id="a1",
            tool="slack",
            action="msg.post",
            idempotency_key="key-big",
            params=huge_params,
        )
        with self.assertRaises(ValidationError) as ctx:
            client.submit_tool_call(req)
        self.assertIn("exceeds", str(ctx.exception))


class TestHTTPErrorMapping(unittest.TestCase):
    """HTTP status codes are mapped to the correct SDK exception types."""

    def _make_client(self) -> OpenClauseClient:
        with patch("openclause.client.requests.Session"):
            return OpenClauseClient(
                base_url="http://localhost:8080",
                api_key="sk-test",
            )

    def _mock_response(self, status: int, body: str = "") -> MagicMock:
        resp = MagicMock()
        resp.status_code = status
        resp.ok = 200 <= status < 300
        resp.text = body
        resp.content = body.encode()
        resp.json.return_value = json.loads(body) if body else {}
        return resp

    def test_401_raises_auth_error(self) -> None:
        client = self._make_client()
        client._session.get.return_value = self._mock_response(401, '"unauthorized"')
        with self.assertRaises(AuthenticationError):
            client.get_event("evt-x")

    def test_500_raises_api_error(self) -> None:
        client = self._make_client()
        client._session.get.return_value = self._mock_response(500, '"server error"')
        with self.assertRaises(APIError) as ctx:
            client.get_event("evt-x")
        self.assertEqual(ctx.exception.status_code, 500)


class TestWaitForApproval(unittest.TestCase):
    """wait_for_approval() retries execute() on 409 "awaiting approval" conflicts."""

    def test_retries_execute_until_approved(self) -> None:
        # Polling with poll_interval=0 avoids sleeping during the test.
        client = OpenClauseClient(base_url="http://localhost:8080", api_key="sk-test")

        approved_resp = ToolCallResponse(
            event_id="evt-1",
            decision="allow",
            reason="approved execution",
        )

        client.execute = MagicMock(  # type: ignore[method-assign]
            side_effect=[
                APIError(409, "awaiting approval"),
                approved_resp,
            ]
        )

        resp = client.wait_for_approval("evt-1", timeout_seconds=1, poll_interval=0)
        self.assertEqual(resp.event_id, "evt-1")
        self.assertEqual(resp.decision, "allow")
        self.assertEqual(client.execute.call_count, 2)

    def test_throws_on_permanent_failure(self) -> None:
        client = OpenClauseClient(base_url="http://localhost:8080", api_key="sk-test")
        client.execute = MagicMock(  # type: ignore[method-assign]
            side_effect=[APIError(403, "forbidden")]
        )

        with self.assertRaises(APIError) as ctx:
            client.wait_for_approval("evt-1", timeout_seconds=1, poll_interval=0)
        self.assertEqual(ctx.exception.status_code, 403)


if __name__ == "__main__":
    unittest.main()
