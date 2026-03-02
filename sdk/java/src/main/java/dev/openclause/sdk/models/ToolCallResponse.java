package dev.openclause.sdk.models;

import com.google.gson.annotations.SerializedName;

public class ToolCallResponse {

    @SerializedName("event_id")
    private String eventId;

    private String decision;
    private String reason;

    @SerializedName("approval_url")
    private String approvalUrl;

    private ExecutionResult result;

    public String getEventId() {
        return eventId;
    }

    public void setEventId(String eventId) {
        this.eventId = eventId;
    }

    public String getDecision() {
        return decision;
    }

    public void setDecision(String decision) {
        this.decision = decision;
    }

    public String getReason() {
        return reason;
    }

    public void setReason(String reason) {
        this.reason = reason;
    }

    public String getApprovalUrl() {
        return approvalUrl;
    }

    public void setApprovalUrl(String approvalUrl) {
        this.approvalUrl = approvalUrl;
    }

    public ExecutionResult getResult() {
        return result;
    }

    public void setResult(ExecutionResult result) {
        this.result = result;
    }
}
