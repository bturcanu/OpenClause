package dev.openclause.sdk.models;

import com.google.gson.Gson;
import com.google.gson.annotations.SerializedName;

/**
 * Full event record returned by {@code GET /v1/toolcalls/{event_id}}.
 */
public class ToolCallEvent {

    @SerializedName("event_id")
    private String eventId;

    @SerializedName("tenant_id")
    private String tenantId;

    @SerializedName("agent_id")
    private String agentId;

    private String tool;
    private String action;
    private String resource;

    @SerializedName("risk_score")
    private Integer riskScore;

    private String decision;
    private String reason;

    private ExecutionResult result;

    @SerializedName("received_at")
    private String receivedAt;

    public String getEventId() { return eventId; }
    public String getTenantId() { return tenantId; }
    public String getAgentId() { return agentId; }
    public String getTool() { return tool; }
    public String getAction() { return action; }
    public String getResource() { return resource; }
    public Integer getRiskScore() { return riskScore; }
    public String getDecision() { return decision; }
    public String getReason() { return reason; }
    public ExecutionResult getResult() { return result; }
    public String getReceivedAt() { return receivedAt; }

    public static ToolCallEvent fromJson(String json) {
        return new Gson().fromJson(json, ToolCallEvent.class);
    }
}
