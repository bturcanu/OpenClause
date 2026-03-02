"""OpenClause gateway SDK client."""

from __future__ import annotations

import json
import time
import uuid

try:
    import requests
except ImportError as exc:
    raise ImportError(
        "The 'requests' package is required. Install it with: pip install requests"
    ) from exc

from .exceptions import (
    APIError,
    AuthenticationError,
    OpenClauseError,
    TimeoutError,
    ValidationError,
)
from .models import ToolCallEvent, ToolCallResponse, ToolCallRequest

_MAX_REQUEST_BYTES = 1 * 1024 * 1024   # 1 MB
_MAX_RESPONSE_BYTES = 4 * 1024 * 1024  # 4 MB
_MAX_POLL_INTERVAL = 30.0


class OpenClauseClient:
    """Synchronous client for the OpenClause governance gateway.

    Args:
        base_url: Root URL of the gateway (e.g. ``http://localhost:8080``).
        api_key:  API key used for authentication.
        timeout:  HTTP request timeout in seconds.
    """

    def __init__(self, base_url: str, api_key: str, timeout: float = 30.0) -> None:
        self._base_url = base_url.rstrip("/")
        self._api_key = api_key
        self._timeout = timeout
        self._session = requests.Session()
        self._session.headers.update({
            "X-API-Key": self._api_key,
            "Content-Type": "application/json",
            "Accept": "application/json",
        })

    # -- public API -----------------------------------------------------------

    def submit_tool_call(self, request: ToolCallRequest) -> ToolCallResponse:
        """Submit a tool call for policy evaluation.

        ``POST /v1/toolcalls``
        """
        body = request.to_dict()
        headers = {}
        if request.trace_id:
            headers["X-Trace-ID"] = request.trace_id

        data = self._post("/v1/toolcalls", body, extra_headers=headers)
        return ToolCallResponse.from_dict(data)

    def get_event(self, event_id: str) -> ToolCallEvent:
        """Retrieve the current state of a tool-call event.

        ``GET /v1/toolcalls/{event_id}``
        """
        data = self._get(f"/v1/toolcalls/{event_id}")
        return ToolCallEvent.from_dict(data)

    def execute(self, event_id: str) -> ToolCallResponse:
        """Execute an approved tool call.

        ``POST /v1/toolcalls/{event_id}/execute``
        """
        data = self._post(f"/v1/toolcalls/{event_id}/execute", body=None)
        return ToolCallResponse.from_dict(data)

    def wait_for_approval(
        self,
        event_id: str,
        timeout_seconds: float = 300,
        poll_interval: float = 2.0,
    ) -> ToolCallResponse:
        """Poll until the event leaves the ``approve`` state, then execute.

        Uses exponential back-off starting from *poll_interval* up to a
        maximum of 30 s between polls.

        Raises:
            TimeoutError: If *timeout_seconds* elapses before a decision.
        """
        deadline = time.monotonic() + timeout_seconds
        interval = poll_interval

        while True:
            event = self.get_event(event_id)
            if event.decision != "approve":
                return ToolCallResponse(
                    event_id=event.event_id,
                    decision=event.decision,
                    reason=event.reason,
                    result=event.result,
                )

            if time.monotonic() >= deadline:
                raise TimeoutError(
                    f"Approval not received within {timeout_seconds}s for event {event_id}"
                )

            time.sleep(min(interval, max(0, deadline - time.monotonic())))
            interval = min(interval * 2, _MAX_POLL_INTERVAL)

    @staticmethod
    def generate_idempotency_key() -> str:
        """Generate a unique idempotency key using UUID v4."""
        return str(uuid.uuid4())

    # -- internal helpers -----------------------------------------------------

    def _post(
        self,
        path: str,
        body: dict | None,
        extra_headers: dict | None = None,
    ) -> dict:
        url = f"{self._base_url}{path}"
        raw_body: bytes | None = None
        if body is not None:
            raw_body = json.dumps(body).encode()
            if len(raw_body) > _MAX_REQUEST_BYTES:
                raise ValidationError(
                    f"Request body ({len(raw_body)} bytes) exceeds the "
                    f"{_MAX_REQUEST_BYTES} byte limit"
                )

        headers = dict(extra_headers) if extra_headers else {}

        try:
            resp = self._session.post(
                url,
                data=raw_body,
                headers=headers,
                timeout=self._timeout,
            )
        except requests.exceptions.Timeout as exc:
            raise TimeoutError(f"Request to {path} timed out") from exc
        except requests.exceptions.RequestException as exc:
            raise OpenClauseError(f"Request failed: {exc}") from exc

        return self._handle_response(resp)

    def _get(self, path: str) -> dict:
        url = f"{self._base_url}{path}"

        try:
            resp = self._session.get(url, timeout=self._timeout)
        except requests.exceptions.Timeout as exc:
            raise TimeoutError(f"Request to {path} timed out") from exc
        except requests.exceptions.RequestException as exc:
            raise OpenClauseError(f"Request failed: {exc}") from exc

        return self._handle_response(resp)

    @staticmethod
    def _handle_response(resp: requests.Response) -> dict:
        content_length = len(resp.content)
        if content_length > _MAX_RESPONSE_BYTES:
            raise ValidationError(
                f"Response body ({content_length} bytes) exceeds the "
                f"{_MAX_RESPONSE_BYTES} byte limit"
            )

        if resp.status_code in (401, 403):
            raise AuthenticationError(
                f"Authentication failed (HTTP {resp.status_code}): {resp.text}"
            )
        if not resp.ok:
            raise APIError(resp.status_code, resp.text)

        return resp.json()
