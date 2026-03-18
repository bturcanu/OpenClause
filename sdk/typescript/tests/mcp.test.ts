import { createMCPToolDefinitions } from "../src/mcp";
import { OpenClauseClient } from "../src/client";

describe("createMCPToolDefinitions", () => {
  it("maps ToolCallResponse.result.output_json to MCP result", async () => {
    const client = {
      submitToolCall: jest.fn().mockResolvedValue({
        event_id: "evt_001",
        decision: "allow",
        reason: "ok",
        result: {
          status: "success",
          output_json: { hello: "world" },
          duration_ms: 10,
        },
      }),
      waitForApproval: jest.fn(),
    } as any as OpenClauseClient;

    const [tool] = createMCPToolDefinitions(client, ["slack.msg.post"], {
      tenantId: "t1",
      agentId: "a1",
    });

    const mcpRes = await tool.execute({
      params: { channel: "#general", text: "hi" },
      resource: "channel/general",
    } as any);

    expect(mcpRes.event_id).toBe("evt_001");
    expect(mcpRes.decision).toBe("allow");
    expect(mcpRes.result).toEqual({ hello: "world" });
    // Ensure it's output payload, not the full execution wrapper.
    expect((mcpRes.result as any).status).toBeUndefined();
  });

  it("waits for approval when decision is approve, then maps output_json", async () => {
    const client = {
      submitToolCall: jest.fn().mockResolvedValue({
        event_id: "evt_002",
        decision: "approve",
        reason: "needs approval",
      }),
      waitForApproval: jest.fn().mockResolvedValue({
        event_id: "evt_002",
        decision: "allow",
        reason: "approved",
        result: {
          status: "success",
          output_json: { ok: true },
          duration_ms: 5,
        },
      }),
    } as any as OpenClauseClient;

    const [tool] = createMCPToolDefinitions(client, ["github.pr.merge"], {
      tenantId: "t1",
      agentId: "a1",
    });

    const mcpRes = await tool.execute({
      params: { pr_number: 123 },
      resource: "repo/1",
    } as any);

    expect(client.waitForApproval).toHaveBeenCalledWith("evt_002");
    expect(mcpRes.decision).toBe("allow");
    expect(mcpRes.result).toEqual({ ok: true });
  });
});

