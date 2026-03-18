/**
 * MCP (Model Context Protocol) server stub for OpenClause.
 *
 * Provides a scaffold for integrating OpenClause as an MCP server
 * that exposes governed tool calls to LLM agents.
 *
 * @example
 * ```ts
 * import { OpenClauseClient } from "./client";
 * import { createMCPToolDefinitions } from "./mcp";
 *
 * const client = new OpenClauseClient({ baseUrl: "http://localhost:8080", apiKey: "sk-oc-..." });
 * const tools = createMCPToolDefinitions(client, ["slack.msg.post", "github.pr.merge"]);
 * ```
 */

import { OpenClauseClient } from "./client";
import { ToolCallResponse } from "./models";

export interface MCPToolDefinition {
  name: string;
  description: string;
  inputSchema: MCPInputSchema;
  execute: (params: Record<string, unknown>) => Promise<MCPToolResult>;
}

export interface MCPInputSchema {
  type: "object";
  properties: Record<string, MCPPropertySchema>;
  required?: string[];
}

export interface MCPPropertySchema {
  type: string;
  description?: string;
}

export interface MCPToolResult {
  event_id: string;
  decision: string;
  reason?: string;
  result?: Record<string, unknown>;
}

export interface MCPToolOptions {
  tenantId?: string;
  agentId?: string;
}

/**
 * Create MCP-compatible tool definitions from a list of tool.action strings.
 *
 * Each entry in `tools` should be formatted as `"toolName.actionName"`.
 *
 * @param client - An initialised OpenClauseClient.
 * @param tools  - List of `"tool.action"` identifiers to expose.
 * @param options - Optional tenant/agent IDs to inject into every request.
 * @returns Array of MCPToolDefinition objects ready to be served.
 */
export function createMCPToolDefinitions(
  client: OpenClauseClient,
  tools: string[],
  options?: MCPToolOptions,
): MCPToolDefinition[] {
  return tools.map((toolAction) => {
    const dotIdx = toolAction.indexOf(".");
    if (dotIdx === -1) {
      throw new Error(
        `Invalid tool spec "${toolAction}": expected "tool.action" format`,
      );
    }

    const toolName = toolAction.slice(0, dotIdx);
    const actionName = toolAction.slice(dotIdx + 1);
    const safeName = `openclause_${toolName}_${actionName}`.replace(
      /[^a-zA-Z0-9_]/g,
      "_",
    );

    return {
      name: safeName,
      description: `Execute ${toolName}.${actionName} via OpenClause governance`,
      inputSchema: {
        type: "object" as const,
        properties: {
          params: {
            type: "object",
            description: `Parameters for ${toolName}.${actionName}`,
          },
          resource: {
            type: "string",
            description: "Target resource identifier",
          },
        },
      },
      execute: async (
        input: Record<string, unknown>,
      ): Promise<MCPToolResult> => {
        const response: ToolCallResponse = await client.submitToolCall({
          tenant_id: options?.tenantId ?? "",
          agent_id: options?.agentId ?? "",
          tool: toolName,
          action: actionName,
          idempotency_key: OpenClauseClient.generateIdempotencyKey(),
          params: (input.params as Record<string, unknown>) ?? {},
          resource: (input.resource as string) ?? undefined,
        });

        let finalResponse = response;
        if (response.decision === "approve") {
          finalResponse = await client.waitForApproval(response.event_id);
        }

        return {
          event_id: finalResponse.event_id,
          decision: finalResponse.decision,
          reason: finalResponse.reason,
          // MCP "result" exposes the tool output payload (output_json) rather than
          // the full execution wrapper.
          result: finalResponse.result?.output_json,
        };
      },
    };
  });
}
