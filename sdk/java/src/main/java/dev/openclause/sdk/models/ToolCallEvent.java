package dev.openclause.sdk.models;

import com.google.gson.Gson;
import com.google.gson.annotations.SerializedName;
import java.util.Map;

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

    private String decision;

    @SerializedName("request")
    private ToolCallRequest request;

    @SerializedName("policy_result")
    private Map<String, Object> policyResult;

    @SerializedName("execution_result")
    private ExecutionResult executionResult;

    @SerializedName("hash")
    private String hash;

    @SerializedName("prev_hash")
    private String prevHash;

    @SerializedName("received_at")
    private String receivedAt;

    private String tool;
    private String action;
    private String resource;

    @SerializedName("risk_score")
    private Integer riskScore;

    @SerializedName("user_id")
    private String userId;

    @SerializedName("session_id")
    private String sessionId;

    @SerializedName("trace_id")
    private String traceId;

    @SerializedName("labels")
    private Map<String, String> labels;

    @SerializedName("source_ip")
    private String sourceIp;

    @SerializedName("requested_at")
    private String requestedAt;

    @SerializedName("reason")
    private String reason;

    private ExecutionResult result;

    public String getEventId() { return eventId; }
    public String getTenantId() { return tenantId; }
    public String getAgentId() { return agentId; }
    public String getDecision() { return decision; }
    public ToolCallRequest getRequest() { return request; }
    public Map<String, Object> getPolicyResult() { return policyResult; }
    public ExecutionResult getExecutionResult() { return executionResult != null ? executionResult : result; }
    public String getHash() { return hash; }
    public String getPrevHash() { return prevHash; }
    public String getReceivedAt() { return receivedAt; }
    public String getTool() { return tool != null ? tool : request != null ? request.getTool() : null; }
    public String getAction() { return action != null ? action : request != null ? request.getAction() : null; }
    public String getResource() { return resource != null ? resource : request != null ? request.getResource() : null; }
    public Integer getRiskScore() { return riskScore != null ? riskScore : request != null ? request.getRiskScore() : null; }
    public String getUserId() { return userId != null ? userId : request != null ? request.getUserId() : null; }
    public String getSessionId() { return sessionId != null ? sessionId : request != null ? request.getSessionId() : null; }
    public String getTraceId() { return traceId != null ? traceId : request != null ? request.getTraceId() : null; }
    public Map<String, String> getLabels() { return labels != null ? labels : request != null ? request.getLabels() : null; }
    public String getSourceIp() { return sourceIp != null ? sourceIp : request != null ? request.getSourceIp() : null; }
    public String getRequestedAt() { return requestedAt != null ? requestedAt : request != null ? request.getRequestedAt() : null; }
    public String getReason() {
        if (reason != null) {
            return reason;
        }
        if (policyResult != null && policyResult.get("reason") instanceof String) {
            return (String) policyResult.get("reason");
        }
        return null;
    }
    public ExecutionResult getResult() { return getExecutionResult(); }

    public static ToolCallEvent fromJson(String json) {
        return new Gson().fromJson(json, ToolCallEvent.class);
    }
}
