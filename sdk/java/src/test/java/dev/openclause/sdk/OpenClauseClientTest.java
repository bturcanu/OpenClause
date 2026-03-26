package dev.openclause.sdk;

import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import com.sun.net.httpserver.HttpServer;
import dev.openclause.sdk.exceptions.APIException;
import dev.openclause.sdk.models.ExecutionResult;
import dev.openclause.sdk.models.ToolCallEvent;
import dev.openclause.sdk.models.ToolCallRequest;
import dev.openclause.sdk.models.ToolCallResponse;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.util.List;
import java.util.Map;
import java.util.HashSet;
import java.util.Set;
import java.util.concurrent.atomic.AtomicReference;
import java.nio.charset.StandardCharsets;

import static org.junit.jupiter.api.Assertions.*;

class OpenClauseClientTest {

    private final Gson gson = new GsonBuilder().create();

    @Test
    void constructorValidatesBaseUrl() {
        assertThrows(IllegalArgumentException.class, () ->
                new OpenClauseClient("", "key"));
    }

    @Test
    void constructorValidatesApiKey() {
        assertThrows(IllegalArgumentException.class, () ->
                new OpenClauseClient("https://api.openclause.dev", ""));
    }

    @Test
    void constructorAcceptsValidArguments() {
        OpenClauseClient client = new OpenClauseClient(
                "https://api.openclause.dev", "test-key");
        assertNotNull(client);
    }

    @Test
    void generateIdempotencyKeyReturnsUUID() {
        String key = OpenClauseClient.generateIdempotencyKey();
        assertNotNull(key);
        assertTrue(key.matches(
                "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"
        ));
    }

    @Test
    void generateIdempotencyKeyReturnsUniqueValues() {
        Set<String> keys = new HashSet<>();
        for (int i = 0; i < 100; i++) {
            keys.add(OpenClauseClient.generateIdempotencyKey());
        }
        assertEquals(100, keys.size());
    }

    @Test
    void serializeMinimalToolCallRequest() {
        ToolCallRequest request = ToolCallRequest.builder(
                "t_123", "agent_abc", "email", "send", "key-001"
        ).build();

        String json = gson.toJson(request);
        assertTrue(json.contains("\"tenant_id\":\"t_123\""));
        assertTrue(json.contains("\"agent_id\":\"agent_abc\""));
        assertTrue(json.contains("\"tool\":\"email\""));
        assertTrue(json.contains("\"action\":\"send\""));
        assertTrue(json.contains("\"idempotency_key\":\"key-001\""));
        assertFalse(json.contains("\"params\""));
    }

    @Test
    void serializeFullToolCallRequest() {
        ToolCallRequest request = ToolCallRequest.builder(
                        "t_123", "agent_abc", "database", "delete", "key-002"
                )
                .params(Map.of("table", "users", "id", 42))
                .resource("users/42")
                .riskScore(8)
                .riskFactors(List.of("destructive", "production"))
                .userId("user_xyz")
                .sessionId("sess_456")
                .labels(Map.of("user_name", "Casey", "user_email", "casey@example.com"))
                .sourceIp("203.0.113.10")
                .traceId("trace_789")
                .requestedAt("2026-01-15T10:30:00Z")
                .schemaVersion("1.0")
                .build();

        String json = gson.toJson(request);
        assertTrue(json.contains("\"resource\":\"users/42\""));
        assertTrue(json.contains("\"risk_score\":8"));
        assertTrue(json.contains("\"user_id\":\"user_xyz\""));
        assertTrue(json.contains("\"session_id\":\"sess_456\""));
        assertTrue(json.contains("\"labels\""));
        assertTrue(json.contains("\"source_ip\":\"203.0.113.10\""));
        assertTrue(json.contains("\"trace_id\":\"trace_789\""));
        assertTrue(json.contains("\"requested_at\":\"2026-01-15T10:30:00Z\""));
        assertTrue(json.contains("\"schema_version\":\"1.0\""));
    }

    @Test
    void riskScoreAcceptsZero() {
        ToolCallRequest request = ToolCallRequest.builder(
                "t_123", "agent_abc", "db", "op", "key-003"
        )
                .riskScore(0)
                .build();

        String json = gson.toJson(request);
        assertTrue(json.contains("\"risk_score\":0"));
    }

    @Test
    void riskScoreAcceptsValidInteger() {
        ToolCallRequest req = ToolCallRequest.builder(
                "t_123", "agent_abc", "db", "op", "key-004"
        ).riskScore(8).build();
        assertEquals(8, req.getRiskScore());
    }

    @Test
    void riskScoreRejectsOutOfRange() {
        assertThrows(IllegalArgumentException.class, () -> ToolCallRequest.builder(
                "t_123", "agent_abc", "db", "op", "key-005"
        ).riskScore(-1).build());

        assertThrows(IllegalArgumentException.class, () -> ToolCallRequest.builder(
                "t_123", "agent_abc", "db", "op", "key-006"
        ).riskScore(11).build());
    }

    @Test
    void deserializeAllowResponse() {
        String json = "{\"event_id\":\"evt_001\",\"decision\":\"allow\",\"reason\":\"Policy matched\"}";
        ToolCallResponse response = gson.fromJson(json, ToolCallResponse.class);

        assertEquals("evt_001", response.getEventId());
        assertEquals("allow", response.getDecision());
        assertEquals("Policy matched", response.getReason());
        assertNull(response.getApprovalUrl());
        assertNull(response.getResult());
    }

    @Test
    void deserializeApproveResponse() {
        String json = "{\"event_id\":\"evt_002\",\"decision\":\"approve\"," +
                "\"approval_url\":\"https://app.openclause.dev/approve/evt_002\"}";
        ToolCallResponse response = gson.fromJson(json, ToolCallResponse.class);

        assertEquals("approve", response.getDecision());
        assertEquals("https://app.openclause.dev/approve/evt_002", response.getApprovalUrl());
    }

    @Test
    void deserializeResponseWithExecutionResult() {
        String json = "{\"event_id\":\"evt_003\",\"decision\":\"allow\"," +
                "\"result\":{\"status\":\"success\",\"output_json\":{\"rows_affected\":1},\"duration_ms\":42}}";
        ToolCallResponse response = gson.fromJson(json, ToolCallResponse.class);

        assertNotNull(response.getResult());
        assertEquals("success", response.getResult().getStatus());
        assertNotNull(response.getResult().getOutputJson());
        assertEquals(42L, response.getResult().getDurationMs());
    }

    @Test
    void deserializeResponseWithExecutionError() {
        String json = "{\"event_id\":\"evt_004\",\"decision\":\"allow\"," +
                "\"result\":{\"status\":\"error\",\"error\":\"connection refused\",\"duration_ms\":1500}}";
        ToolCallResponse response = gson.fromJson(json, ToolCallResponse.class);

        assertEquals("error", response.getResult().getStatus());
        assertEquals("connection refused", response.getResult().getError());
    }

    @Test
    void toolCallRequestBuilderSetsRequiredFields() {
        ToolCallRequest request = ToolCallRequest.builder(
                "tenant", "agent", "tool", "action", "key"
        ).build();

        assertEquals("tenant", request.getTenantId());
        assertEquals("agent", request.getAgentId());
        assertEquals("tool", request.getTool());
        assertEquals("action", request.getAction());
        assertEquals("key", request.getIdempotencyKey());
    }

    @Test
    void toolCallRequestBuilderChainsOptionalFields() {
        ToolCallRequest request = ToolCallRequest.builder(
                        "t", "a", "tool", "act", "k"
                )
                .resource("res")
                .userId("u")
                .sessionId("s")
                .labels(Map.of("user_name", "Casey"))
                .sourceIp("203.0.113.10")
                .traceId("tr")
                .requestedAt("2026-01-15T10:30:00Z")
                .build();

        assertEquals("res", request.getResource());
        assertEquals("u", request.getUserId());
        assertEquals("s", request.getSessionId());
        assertEquals("203.0.113.10", request.getSourceIp());
        assertEquals("2026-01-15T10:30:00Z", request.getRequestedAt());
        assertEquals("tr", request.getTraceId());
    }

    @Test
    void deserializeToolCallEnvelopePreservesLinkedRequestMetadata() {
        String json = "{"
                + "\"event_id\":\"evt_005\","
                + "\"request\":{"
                + "\"tenant_id\":\"t_123\","
                + "\"agent_id\":\"agent_abc\","
                + "\"tool\":\"slack\","
                + "\"action\":\"msg.post\","
                + "\"idempotency_key\":\"key-005\","
                + "\"session_id\":\"sess_456\","
                + "\"trace_id\":\"trace_789\","
                + "\"labels\":{\"user_name\":\"Casey\",\"user_email\":\"casey@example.com\"},"
                + "\"requested_at\":\"2026-01-15T10:30:00Z\""
                + "},"
                + "\"decision\":\"allow\","
                + "\"policy_result\":{\"decision\":\"allow\",\"reason\":\"ok\"},"
                + "\"execution_result\":{\"status\":\"success\",\"duration_ms\":12},"
                + "\"hash\":\"hash-1\","
                + "\"prev_hash\":\"hash-0\","
                + "\"received_at\":\"2026-01-15T10:31:00Z\""
                + "}";

        ToolCallEvent event = gson.fromJson(json, ToolCallEvent.class);
        assertNotNull(event.getRequest());
        assertEquals("sess_456", event.getSessionId());
        assertEquals("trace_789", event.getTraceId());
        assertEquals("slack", event.getTool());
        assertEquals("msg.post", event.getAction());
        assertEquals("success", event.getResult().getStatus());
        assertEquals("ok", event.getReason());
        assertEquals("hash-1", event.getHash());
        assertEquals("2026-01-15T10:31:00Z", event.getReceivedAt());
    }

    @Test
    void deserializeToolCallEventWithoutNestedRequestUsesTopLevelFields() {
        String json = "{"
                + "\"event_id\":\"evt_006\","
                + "\"decision\":\"allow\","
                + "\"tenant_id\":\"tenant-1\","
                + "\"agent_id\":\"agent-1\","
                + "\"tool\":\"slack\","
                + "\"action\":\"msg.post\","
                + "\"resource\":\"channels/general\","
                + "\"risk_score\":2,"
                + "\"user_id\":\"user-1\","
                + "\"session_id\":\"sess-1\","
                + "\"trace_id\":\"trace-1\","
                + "\"labels\":{\"user_name\":\"Casey\"},"
                + "\"source_ip\":\"203.0.113.10\","
                + "\"requested_at\":\"2026-01-15T10:30:00Z\","
                + "\"policy_result\":{\"decision\":\"allow\",\"reason\":\"ok\"},"
                + "\"result\":{\"status\":\"success\",\"duration_ms\":12},"
                + "\"received_at\":\"2026-01-15T10:31:00Z\""
                + "}";

        ToolCallEvent event = gson.fromJson(json, ToolCallEvent.class);
        assertEquals("tenant-1", event.getTenantId());
        assertEquals("agent-1", event.getAgentId());
        assertEquals("slack", event.getTool());
        assertEquals("msg.post", event.getAction());
        assertEquals("channels/general", event.getResource());
        assertEquals(2, event.getRiskScore());
        assertEquals("user-1", event.getUserId());
        assertEquals("sess-1", event.getSessionId());
        assertEquals("trace-1", event.getTraceId());
        assertEquals("203.0.113.10", event.getSourceIp());
        assertEquals("2026-01-15T10:30:00Z", event.getRequestedAt());
        assertEquals("success", event.getResult().getStatus());
        assertEquals("ok", event.getReason());
    }

    @Test
    void submitToolCallSendsApiKeyAndJsonBody() throws Exception {
        AtomicReference<String> apiKeyHeader = new AtomicReference<>();
        AtomicReference<String> bodyRef = new AtomicReference<>();
        HttpServer server = startServer(exchange -> {
            apiKeyHeader.set(exchange.getRequestHeaders().getFirst("X-API-Key"));
            bodyRef.set(new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8));
            byte[] body = "{\"event_id\":\"evt-1\",\"decision\":\"allow\"}".getBytes(StandardCharsets.UTF_8);
            exchange.getResponseHeaders().add("Content-Type", "application/json");
            exchange.sendResponseHeaders(200, body.length);
            exchange.getResponseBody().write(body);
            exchange.close();
        }, "/v1/toolcalls");

        try {
            OpenClauseClient client = new OpenClauseClient(baseUrl(server), "test-key");
            ToolCallRequest request = ToolCallRequest.builder(
                    "tenant-1", "agent-1", "slack", "msg.post", "key-1"
            ).traceId("trace-1").build();

            ToolCallResponse response = client.submitToolCall(request);
            assertEquals("evt-1", response.getEventId());
            assertEquals("test-key", apiKeyHeader.get());
            assertTrue(bodyRef.get().contains("\"tenant_id\":\"tenant-1\""));
            assertTrue(bodyRef.get().contains("\"trace_id\":\"trace-1\""));
        } finally {
            server.stop(0);
        }
    }

    @Test
    void getEventMaps401ToApiException() throws Exception {
        HttpServer server = startServer(exchange -> {
            byte[] body = "{\"message\":\"bad key\"}".getBytes(StandardCharsets.UTF_8);
            exchange.sendResponseHeaders(401, body.length);
            exchange.getResponseBody().write(body);
            exchange.close();
        }, "/v1/toolcalls/evt-auth");

        try {
            OpenClauseClient client = new OpenClauseClient(baseUrl(server), "test-key");
            APIException err = assertThrows(APIException.class, () -> client.getEvent("evt-auth"));
            assertEquals(401, err.getStatusCode());
            assertTrue(err.getResponseBody().contains("bad key"));
        } finally {
            server.stop(0);
        }
    }

    @Test
    void executeMaps500ToApiException() throws Exception {
        HttpServer server = startServer(exchange -> {
            byte[] body = "{\"message\":\"server error\"}".getBytes(StandardCharsets.UTF_8);
            exchange.sendResponseHeaders(500, body.length);
            exchange.getResponseBody().write(body);
            exchange.close();
        }, "/v1/toolcalls/evt-500/execute");

        try {
            OpenClauseClient client = new OpenClauseClient(baseUrl(server), "test-key");
            APIException err = assertThrows(APIException.class, () -> client.execute("evt-500"));
            assertEquals(500, err.getStatusCode());
            assertTrue(err.getResponseBody().contains("server error"));
        } finally {
            server.stop(0);
        }
    }

    private static HttpServer startServer(ThrowingHttpHandler handler, String path) throws IOException {
        HttpServer server = HttpServer.create(new InetSocketAddress(0), 0);
        server.createContext(path, exchange -> {
            try {
                handler.handle(exchange);
            } catch (Exception e) {
                throw new IOException(e);
            }
        });
        server.start();
        return server;
    }

    private static String baseUrl(HttpServer server) {
        return "http://localhost:" + server.getAddress().getPort();
    }

    @FunctionalInterface
    private interface ThrowingHttpHandler {
        void handle(com.sun.net.httpserver.HttpExchange exchange) throws Exception;
    }
}
