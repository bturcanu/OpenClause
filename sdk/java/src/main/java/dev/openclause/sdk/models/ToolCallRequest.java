package dev.openclause.sdk.models;

import com.google.gson.annotations.SerializedName;
import java.util.List;
import java.util.Map;

public class ToolCallRequest {

    @SerializedName("tenant_id")
    private String tenantId;

    @SerializedName("agent_id")
    private String agentId;

    private String tool;
    private String action;

    @SerializedName("idempotency_key")
    private String idempotencyKey;

    private Map<String, Object> params;
    private String resource;

    @SerializedName("risk_score")
    private Double riskScore;

    @SerializedName("risk_factors")
    private List<String> riskFactors;

    @SerializedName("user_id")
    private String userId;

    @SerializedName("session_id")
    private String sessionId;

    @SerializedName("trace_id")
    private String traceId;

    @SerializedName("schema_version")
    private String schemaVersion;

    private ToolCallRequest() {}

    public String getTenantId() { return tenantId; }
    public String getAgentId() { return agentId; }
    public String getTool() { return tool; }
    public String getAction() { return action; }
    public String getIdempotencyKey() { return idempotencyKey; }
    public Map<String, Object> getParams() { return params; }
    public String getResource() { return resource; }
    public Double getRiskScore() { return riskScore; }
    public List<String> getRiskFactors() { return riskFactors; }
    public String getUserId() { return userId; }
    public String getSessionId() { return sessionId; }
    public String getTraceId() { return traceId; }
    public String getSchemaVersion() { return schemaVersion; }

    public static Builder builder(String tenantId, String agentId, String tool, String action, String idempotencyKey) {
        return new Builder(tenantId, agentId, tool, action, idempotencyKey);
    }

    public static class Builder {
        private final ToolCallRequest request;

        private Builder(String tenantId, String agentId, String tool, String action, String idempotencyKey) {
            request = new ToolCallRequest();
            request.tenantId = tenantId;
            request.agentId = agentId;
            request.tool = tool;
            request.action = action;
            request.idempotencyKey = idempotencyKey;
        }

        public Builder params(Map<String, Object> params) {
            request.params = params;
            return this;
        }

        public Builder resource(String resource) {
            request.resource = resource;
            return this;
        }

        public Builder riskScore(double riskScore) {
            request.riskScore = riskScore;
            return this;
        }

        public Builder riskFactors(List<String> riskFactors) {
            request.riskFactors = riskFactors;
            return this;
        }

        public Builder userId(String userId) {
            request.userId = userId;
            return this;
        }

        public Builder sessionId(String sessionId) {
            request.sessionId = sessionId;
            return this;
        }

        public Builder traceId(String traceId) {
            request.traceId = traceId;
            return this;
        }

        public Builder schemaVersion(String schemaVersion) {
            request.schemaVersion = schemaVersion;
            return this;
        }

        public ToolCallRequest build() {
            return request;
        }
    }
}
