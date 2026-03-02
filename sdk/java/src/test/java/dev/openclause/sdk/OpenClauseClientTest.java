package dev.openclause.sdk;

import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import dev.openclause.sdk.models.ExecutionResult;
import dev.openclause.sdk.models.ToolCallRequest;
import dev.openclause.sdk.models.ToolCallResponse;
import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Map;
import java.util.HashSet;
import java.util.Set;

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
                .riskScore(0.95)
                .riskFactors(List.of("destructive", "production"))
                .userId("user_xyz")
                .sessionId("sess_456")
                .traceId("trace_789")
                .schemaVersion("1.0")
                .build();

        String json = gson.toJson(request);
        assertTrue(json.contains("\"resource\":\"users/42\""));
        assertTrue(json.contains("\"risk_score\":0.95"));
        assertTrue(json.contains("\"user_id\":\"user_xyz\""));
        assertTrue(json.contains("\"session_id\":\"sess_456\""));
        assertTrue(json.contains("\"trace_id\":\"trace_789\""));
        assertTrue(json.contains("\"schema_version\":\"1.0\""));
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
                .traceId("tr")
                .build();

        assertEquals("res", request.getResource());
        assertEquals("u", request.getUserId());
        assertEquals("s", request.getSessionId());
        assertEquals("tr", request.getTraceId());
    }
}
