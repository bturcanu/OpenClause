"""LangChain integration for OpenClause.

Usage::

    from openclause import OpenClauseClient
    from openclause.langchain import OpenClauseTool

    client = OpenClauseClient(base_url="http://localhost:8080", api_key="sk-oc-...")

    tool = OpenClauseTool(client=client, tool_name="slack", action="msg.post")
    result = tool._run({"channel": "#general", "text": "hello"})
"""

from __future__ import annotations

import asyncio
import json
from typing import Any

from .client import OpenClauseClient
from .models import ToolCallRequest


class OpenClauseTool:
    """Wraps an OpenClause tool call as a LangChain-compatible tool.

    Implements the minimal interface expected by LangChain's ``BaseTool``
    (``name``, ``description``, ``_run``) so it can be dropped into an
    agent's tool list without inheriting from LangChain directly.

    Attributes:
        name: Unique tool identifier used by the LLM agent.
        description: Human-readable description shown to the agent.
    """

    name: str
    description: str

    def __init__(
        self,
        client: OpenClauseClient,
        tool_name: str,
        action: str,
        *,
        tenant_id: str = "",
        agent_id: str = "",
        resource: str = "",
        description: str | None = None,
    ) -> None:
        self.client = client
        self.tool_name = tool_name
        self.action = action
        self.tenant_id = tenant_id
        self.agent_id = agent_id
        self.resource = resource
        self.name = f"openclause_{tool_name}_{action}"
        self.description = (
            description
            or f"Execute {tool_name}.{action} via OpenClause governance"
        )

    def _run(self, params: dict[str, Any] | str | None = None) -> str:
        """Execute the governed tool call synchronously.

        Args:
            params: Tool parameters forwarded to the gateway. Accepts a dict
                or a JSON string (LangChain sometimes passes raw strings).

        Returns:
            JSON-encoded decision response from OpenClause.
        """
        if isinstance(params, str):
            params = json.loads(params) if params else {}

        request = ToolCallRequest(
            tenant_id=self.tenant_id,
            agent_id=self.agent_id,
            tool=self.tool_name,
            action=self.action,
            idempotency_key=OpenClauseClient.generate_idempotency_key(),
            params=params,
            resource=self.resource,
        )
        response = self.client.submit_tool_call(request)

        if response.decision == "approve":
            response = self.client.wait_for_approval(response.event_id)

        result: dict[str, Any] = {
            "event_id": response.event_id,
            "decision": response.decision,
            "reason": response.reason,
        }
        if response.result:
            result["result"] = {
                "status": response.result.status,
                "output_json": response.result.output_json,
                "error": response.result.error,
            }
        return json.dumps(result)

    async def _arun(self, params: dict[str, Any] | str | None = None) -> str:
        """Async variant — runs the sync implementation in a thread to avoid blocking the event loop."""
        return await asyncio.to_thread(self._run, params)
