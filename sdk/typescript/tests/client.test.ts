import { OpenClauseClient } from "../src/client";
import { ToolCallRequest, ToolCallResponse } from "../src/models";
import { OpenClauseError, APIError, AuthenticationError, TimeoutError } from "../src/errors";

function mockJSONResponse(
  payload: unknown,
  options: {
    status?: number;
    statusText?: string;
    headers?: Record<string, string>;
  } = {},
) {
  const status = options.status ?? 200;
  const normalizedHeaders = new Map(
    Object.entries(options.headers ?? {}).map(([key, value]) => [key.toLowerCase(), value]),
  );
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: options.statusText ?? "OK",
    headers: {
      get: (name: string) => normalizedHeaders.get(name.toLowerCase()) ?? null,
    },
    text: async () => JSON.stringify(payload),
  };
}

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

    it("should strip trailing slashes from baseUrl", async () => {
      const fetchMock = jest.fn().mockResolvedValue(
        mockJSONResponse({ event_id: "evt-trimmed", decision: "allow" }),
      );
      (global as any).fetch = fetchMock;

      const client = new OpenClauseClient({ baseUrl: BASE_URL + "///", apiKey: API_KEY });
      await client.getEvent("evt-trimmed");

      expect(fetchMock).toHaveBeenCalledWith(
        `${BASE_URL}/v1/toolcalls/evt-trimmed`,
        expect.objectContaining({ method: "GET" }),
      );
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

  describe("submitToolCall", () => {
    it("submits a minimal request body without optional fields", async () => {
      const fetchMock = jest.fn().mockResolvedValue(
        mockJSONResponse({ event_id: "evt-minimal", decision: "allow" }),
      );
      (global as any).fetch = fetchMock;

      const request: ToolCallRequest = {
        tenant_id: "t_123",
        agent_id: "agent_abc",
        tool: "email",
        action: "send",
        idempotency_key: "key-001",
      };

      const client = new OpenClauseClient({ baseUrl: BASE_URL, apiKey: API_KEY });
      const response = await client.submitToolCall(request);
      const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
      const parsed = JSON.parse(String(init.body));

      expect(response.event_id).toBe("evt-minimal");
      expect(url).toBe(`${BASE_URL}/v1/toolcalls`);
      expect(init.headers).toMatchObject({
        "Content-Type": "application/json",
        "X-API-Key": API_KEY,
      });
      expect(parsed.tenant_id).toBe("t_123");
      expect(parsed.agent_id).toBe("agent_abc");
      expect(parsed.tool).toBe("email");
      expect(parsed.action).toBe("send");
      expect(parsed.idempotency_key).toBe("key-001");
      expect(parsed.params).toBeUndefined();
    });

    it("submits optional request fields and parses approval responses", async () => {
      const fetchMock = jest.fn().mockResolvedValue(
        mockJSONResponse({
          event_id: "evt-approve",
          decision: "approve",
          approval_url: "https://app.openclause.dev/approve/evt-approve",
        }),
      );
      (global as any).fetch = fetchMock;

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
        labels: { user_name: "Casey", user_email: "casey@example.com" },
        source_ip: "203.0.113.10",
        trace_id: "trace_789",
        requested_at: "2026-01-15T10:30:00Z",
        schema_version: "1.0",
      };

      const client = new OpenClauseClient({ baseUrl: BASE_URL, apiKey: API_KEY });
      const response = await client.submitToolCall(request);
      const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
      const parsed = JSON.parse(String(init.body));

      expect(parsed.params).toEqual({ table: "users", id: 42 });
      expect(parsed.resource).toBe("users/42");
      expect(parsed.risk_score).toBe(8);
      expect(parsed.risk_factors).toEqual(["destructive", "production"]);
      expect(parsed.user_id).toBe("user_xyz");
      expect(parsed.session_id).toBe("sess_456");
      expect(parsed.labels).toEqual({ user_name: "Casey", user_email: "casey@example.com" });
      expect(parsed.source_ip).toBe("203.0.113.10");
      expect(parsed.trace_id).toBe("trace_789");
      expect(parsed.requested_at).toBe("2026-01-15T10:30:00Z");
      expect(parsed.schema_version).toBe("1.0");
      expect(response.decision).toBe("approve");
      expect(response.approval_url).toBe("https://app.openclause.dev/approve/evt-approve");
    });

    it("includes risk_score when it is explicitly set to zero", async () => {
      const fetchMock = jest.fn().mockResolvedValue(
        mockJSONResponse({
          event_id: "evt-zero-risk",
          decision: "allow",
          reason: "risk score included",
        }),
      );
      (global as any).fetch = fetchMock;

      const request: ToolCallRequest = {
        tenant_id: "t_123",
        agent_id: "agent_abc",
        tool: "slack",
        action: "msg.post",
        idempotency_key: "key-000",
        risk_score: 0,
      };

      const client = new OpenClauseClient({ baseUrl: BASE_URL, apiKey: API_KEY });
      await client.submitToolCall(request);
      const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
      const parsed = JSON.parse(String(init.body));

      expect(parsed.risk_score).toBe(0);
    });

    it("parses execution results returned by the API", async () => {
      const fetchMock = jest.fn().mockResolvedValue(
        mockJSONResponse({
          event_id: "evt_003",
          decision: "allow",
          result: {
            status: "success",
            output_json: { rows_affected: 1 },
            duration_ms: 42,
          },
        }),
      );
      (global as any).fetch = fetchMock;

      const client = new OpenClauseClient({ baseUrl: BASE_URL, apiKey: API_KEY });
      const response = await client.submitToolCall({
        tenant_id: "t_123",
        agent_id: "agent_abc",
        tool: "database",
        action: "read",
        idempotency_key: "key-003",
      });

      expect(response.result).toBeDefined();
      expect(response.result!.status).toBe("success");
      expect(response.result!.output_json).toEqual({ rows_affected: 1 });
      expect(response.result!.duration_ms).toBe(42);
      expect(response.result!.error).toBeUndefined();
    });

    it("parses execution errors returned by the API", async () => {
      const fetchMock = jest.fn().mockResolvedValue(
        mockJSONResponse({
          event_id: "evt_004",
          decision: "allow",
          result: {
            status: "error",
            error: "connection refused",
            duration_ms: 1500,
          },
        }),
      );
      (global as any).fetch = fetchMock;

      const client = new OpenClauseClient({ baseUrl: BASE_URL, apiKey: API_KEY });
      const response = await client.execute("evt_004");

      expect(response.result!.status).toBe("error");
      expect(response.result!.error).toBe("connection refused");
    });
  });

  describe("getEvent", () => {
    it("preserves the nested envelope returned by GET /v1/toolcalls/{event_id}", async () => {
      const fetchMock = jest.fn().mockResolvedValue(
        mockJSONResponse({
          event_id: "evt_005",
          request: {
            tenant_id: "t_123",
          agent_id: "agent_abc",
          tool: "slack",
          action: "msg.post",
          idempotency_key: "key-005",
          session_id: "sess_456",
          trace_id: "trace_789",
          labels: { user_name: "Casey", user_email: "casey@example.com" },
          requested_at: "2026-01-15T10:30:00Z",
        },
        decision: "allow",
        policy_result: { decision: "allow", reason: "ok" },
        execution_result: { status: "success", duration_ms: 12 },
        hash: "hash-1",
        prev_hash: "hash-0",
        received_at: "2026-01-15T10:31:00Z",
        }),
      );
      (global as any).fetch = fetchMock;

      const client = new OpenClauseClient({ baseUrl: BASE_URL, apiKey: API_KEY });
      const event = await client.getEvent("evt_005");

      expect(event.event_id).toBe("evt_005");
      expect(event.request.session_id).toBe("sess_456");
      expect(event.request.trace_id).toBe("trace_789");
      expect(event.request.labels).toEqual({
        user_name: "Casey",
        user_email: "casey@example.com",
      });
      expect(event.execution_result?.status).toBe("success");
      expect(event.hash).toBe("hash-1");
      expect(event.received_at).toBe("2026-01-15T10:31:00Z");
      expect(fetchMock).toHaveBeenCalledWith(
        `${BASE_URL}/v1/toolcalls/evt_005`,
        expect.objectContaining({ method: "GET" }),
      );
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

    it("getEvent parses top-level event metadata and execution_result", async () => {
      const fetchMock = jest.fn().mockResolvedValue({
        ok: true,
        status: 200,
        statusText: "OK",
        headers: { get: () => null },
        text: async () => JSON.stringify({
          event_id: "evt-6",
          decision: "allow",
          tenant_id: "tenant-1",
          agent_id: "agent-1",
          tool: "slack",
          action: "msg.post",
          resource: "channels/general",
          risk_score: 2,
          user_id: "user-1",
          session_id: "sess-1",
          trace_id: "trace-1",
          source_ip: "203.0.113.10",
          requested_at: "2026-01-15T10:30:00Z",
          policy_result: { decision: "allow", reason: "ok" },
          execution_result: { status: "success", duration_ms: 12, output_json: { ok: true } },
          received_at: "2026-01-15T10:31:00Z",
        }),
      });
      (global as any).fetch = fetchMock;

      const client = new OpenClauseClient({ baseUrl: BASE_URL, apiKey: API_KEY });
      const event = await client.getEvent("evt-6");

      expect(event.event_id).toBe("evt-6");
      expect(event.decision).toBe("allow");
      expect(event.tenant_id).toBe("tenant-1");
      expect(event.requested_at).toBe("2026-01-15T10:30:00Z");
      expect(event.execution_result?.status).toBe("success");
      expect(event.policy_result).toEqual({ decision: "allow", reason: "ok" });
      expect(fetchMock).toHaveBeenCalledWith(
        `${BASE_URL}/v1/toolcalls/evt-6`,
        expect.objectContaining({
          method: "GET",
        }),
      );
    });

    it("maps abort errors to TimeoutError", async () => {
      const abortError = new DOMException("aborted", "AbortError");
      (global as any).fetch = jest.fn().mockRejectedValue(abortError);

      const client = new OpenClauseClient({ baseUrl: BASE_URL, apiKey: API_KEY, timeout: 1 });

      await expect(client.getEvent("evt-timeout")).rejects.toBeInstanceOf(TimeoutError);
    });
  });
});
