import { OpenClauseClient } from "../src/client";
import { ToolCallRequest, ToolCallResponse } from "../src/models";
import { OpenClauseError, APIError, AuthenticationError, TimeoutError } from "../src/errors";

describe("OpenClauseClient", () => {
  const BASE_URL = "https://api.openclause.dev";
  const API_KEY = "test-api-key-123";

  afterEach(() => {
    jest.restoreAllMocks();
  });

  describe("constructor", () => {
    it("should create a client with valid options", () => {
      const client = new OpenClauseClient({ baseUrl: BASE_URL, apiKey: API_KEY });
      expect(client).toBeInstanceOf(OpenClauseClient);
    });

    it("should throw if baseUrl is empty", () => {
      expect(() => new OpenClauseClient({ baseUrl: "", apiKey: API_KEY }))
        .toThrow(OpenClauseError);
    });

    it("should throw if apiKey is empty", () => {
      expect(() => new OpenClauseClient({ baseUrl: BASE_URL, apiKey: "" }))
        .toThrow(OpenClauseError);
    });

    it("should strip trailing slashes from baseUrl", () => {
      const client = new OpenClauseClient({ baseUrl: BASE_URL + "///", apiKey: API_KEY });
      expect(client).toBeInstanceOf(OpenClauseClient);
    });
  });

  describe("generateIdempotencyKey", () => {
    it("should return a valid UUID string", () => {
      const key = OpenClauseClient.generateIdempotencyKey();
      expect(key).toMatch(
        /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/,
      );
    });

    it("should return unique keys on each call", () => {
      const keys = new Set(
        Array.from({ length: 100 }, () => OpenClauseClient.generateIdempotencyKey()),
      );
      expect(keys.size).toBe(100);
    });
  });

  describe("ToolCallRequest serialization", () => {
    it("should serialize a minimal request correctly", () => {
      const request: ToolCallRequest = {
        tenant_id: "t_123",
        agent_id: "agent_abc",
        tool: "email",
        action: "send",
        idempotency_key: "key-001",
      };

      const json = JSON.stringify(request);
      const parsed = JSON.parse(json);

      expect(parsed.tenant_id).toBe("t_123");
      expect(parsed.agent_id).toBe("agent_abc");
      expect(parsed.tool).toBe("email");
      expect(parsed.action).toBe("send");
      expect(parsed.idempotency_key).toBe("key-001");
      expect(parsed.params).toBeUndefined();
    });

    it("should serialize a full request with all optional fields", () => {
      const request: ToolCallRequest = {
        tenant_id: "t_123",
        agent_id: "agent_abc",
        tool: "database",
        action: "delete",
        idempotency_key: "key-002",
        params: { table: "users", id: 42 },
        resource: "users/42",
        risk_score: 8,
        risk_factors: ["destructive", "production"],
        user_id: "user_xyz",
        session_id: "sess_456",
        trace_id: "trace_789",
        schema_version: "1.0",
      };

      const json = JSON.stringify(request);
      const parsed = JSON.parse(json);

      expect(parsed.params).toEqual({ table: "users", id: 42 });
      expect(parsed.resource).toBe("users/42");
      expect(parsed.risk_score).toBe(8);
      expect(parsed.risk_factors).toEqual(["destructive", "production"]);
      expect(parsed.user_id).toBe("user_xyz");
      expect(parsed.session_id).toBe("sess_456");
      expect(parsed.trace_id).toBe("trace_789");
      expect(parsed.schema_version).toBe("1.0");
    });
  });

  describe("ToolCallResponse deserialization", () => {
    it("should deserialize an allow response", () => {
      const raw = JSON.stringify({
        event_id: "evt_001",
        decision: "allow",
        reason: "Policy matched",
      });

      const response: ToolCallResponse = JSON.parse(raw);
      expect(response.event_id).toBe("evt_001");
      expect(response.decision).toBe("allow");
      expect(response.reason).toBe("Policy matched");
      expect(response.approval_url).toBeUndefined();
      expect(response.result).toBeUndefined();
    });

    it("should deserialize an approve response with approval_url", () => {
      const raw = JSON.stringify({
        event_id: "evt_002",
        decision: "approve",
        approval_url: "https://app.openclause.dev/approve/evt_002",
      });

      const response: ToolCallResponse = JSON.parse(raw);
      expect(response.decision).toBe("approve");
      expect(response.approval_url).toBe(
        "https://app.openclause.dev/approve/evt_002",
      );
    });

    it("should deserialize a response with execution result", () => {
      const raw = JSON.stringify({
        event_id: "evt_003",
        decision: "allow",
        result: {
          status: "success",
          output_json: { rows_affected: 1 },
          duration_ms: 42,
        },
      });

      const response: ToolCallResponse = JSON.parse(raw);
      expect(response.result).toBeDefined();
      expect(response.result!.status).toBe("success");
      expect(response.result!.output_json).toEqual({ rows_affected: 1 });
      expect(response.result!.duration_ms).toBe(42);
      expect(response.result!.error).toBeUndefined();
    });

    it("should deserialize a response with execution error", () => {
      const raw = JSON.stringify({
        event_id: "evt_004",
        decision: "allow",
        result: {
          status: "error",
          error: "connection refused",
          duration_ms: 1500,
        },
      });

      const response: ToolCallResponse = JSON.parse(raw);
      expect(response.result!.status).toBe("error");
      expect(response.result!.error).toBe("connection refused");
    });
  });

  describe("waitForApproval", () => {
    it("retries execute on 409 awaiting approval and returns execution response", async () => {
      const client = new OpenClauseClient({ baseUrl: BASE_URL, apiKey: API_KEY });

      const awaitingApprovalErr = new APIError(
        "Conflict: awaiting approval",
        409,
        '{"code":"CONFLICT","message":"awaiting approval"}',
      );

      const executionResponse: ToolCallResponse = {
        event_id: "evt_approve_1",
        decision: "allow",
        result: { status: "success", duration_ms: 10, output_json: {} },
      };

      const executeSpy = jest
        .spyOn(client, "execute")
        .mockRejectedValueOnce(awaitingApprovalErr)
        .mockResolvedValueOnce(executionResponse);

      const res = await client.waitForApproval("evt_approve_1", {
        timeoutMs: 5_000,
        pollIntervalMs: 0,
      });

      expect(executeSpy).toHaveBeenCalledTimes(2);
      expect(res.decision).toBe("allow");
      expect(res.result?.status).toBe("success");
    });

    it("throws immediately on permanent failures (400/403/404)", async () => {
      const client = new OpenClauseClient({ baseUrl: BASE_URL, apiKey: API_KEY });

      const permErr = new APIError(
        "Forbidden",
        403,
        '{"code":"FORBIDDEN","message":"no permission"}',
      );

      jest.spyOn(client, "execute").mockRejectedValueOnce(permErr);

      await expect(
        client.waitForApproval("evt_perm_fail", { timeoutMs: 5_000, pollIntervalMs: 0 }),
      ).rejects.toBe(permErr);
    });
  });

  describe("error classes", () => {
    it("should create an APIError with status code", () => {
      const err = new APIError("Not Found", 404, '{"error":"not found"}');
      expect(err).toBeInstanceOf(OpenClauseError);
      expect(err).toBeInstanceOf(APIError);
      expect(err.statusCode).toBe(404);
      expect(err.responseBody).toBe('{"error":"not found"}');
      expect(err.message).toBe("Not Found");
      expect(err.name).toBe("APIError");
    });

    it("should create an AuthenticationError", () => {
      const err = new AuthenticationError();
      expect(err).toBeInstanceOf(OpenClauseError);
      expect(err.name).toBe("AuthenticationError");
    });

    it("should create a TimeoutError", () => {
      const err = new TimeoutError("custom timeout");
      expect(err).toBeInstanceOf(OpenClauseError);
      expect(err.name).toBe("TimeoutError");
      expect(err.message).toBe("custom timeout");
    });
  });

  describe("HTTP request behavior", () => {
    it("submitToolCall builds the request with X-API-Key and JSON body", async () => {
      const fetchMock = jest.fn().mockResolvedValue({
        ok: true,
        status: 200,
        statusText: "OK",
        headers: { get: () => null },
        text: async () => JSON.stringify({ event_id: "evt_1", decision: "allow" }),
      });
      (global as any).fetch = fetchMock;

      const client = new OpenClauseClient({ baseUrl: BASE_URL, apiKey: API_KEY });
      const req: ToolCallRequest = {
        tenant_id: "tenant-1",
        agent_id: "agent-1",
        tool: "slack",
        action: "msg.post",
        idempotency_key: "idem-1",
        trace_id: "trace-1",
      };

      const res = await client.submitToolCall(req);

      expect(res.event_id).toBe("evt_1");
      expect(fetchMock).toHaveBeenCalledTimes(1);
      const [url, init] = fetchMock.mock.calls[0];
      expect(url).toBe(`${BASE_URL}/v1/toolcalls`);
      expect(init.method).toBe("POST");
      expect(init.headers).toMatchObject({
        "Content-Type": "application/json",
        "X-API-Key": API_KEY,
      });
      expect(JSON.parse(init.body)).toEqual(req);
    });

    it("getEvent maps 401 responses to AuthenticationError", async () => {
      (global as any).fetch = jest.fn().mockResolvedValue({
        ok: false,
        status: 401,
        statusText: "Unauthorized",
        headers: { get: () => null },
        text: async () => '{"message":"bad key"}',
      });

      const client = new OpenClauseClient({ baseUrl: BASE_URL, apiKey: API_KEY });

      await expect(client.getEvent("evt-auth")).rejects.toBeInstanceOf(AuthenticationError);
    });

    it("maps abort errors to TimeoutError", async () => {
      const abortError = new DOMException("aborted", "AbortError");
      (global as any).fetch = jest.fn().mockRejectedValue(abortError);

      const client = new OpenClauseClient({ baseUrl: BASE_URL, apiKey: API_KEY, timeout: 1 });

      await expect(client.getEvent("evt-timeout")).rejects.toBeInstanceOf(TimeoutError);
    });
  });
});
