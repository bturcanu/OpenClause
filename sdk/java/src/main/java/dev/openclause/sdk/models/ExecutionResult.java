package dev.openclause.sdk.models;

import com.google.gson.annotations.SerializedName;
import java.util.Map;

public class ExecutionResult {

    private String status;

    @SerializedName("output_json")
    private Map<String, Object> outputJson;

    private String error;

    @SerializedName("duration_ms")
    private long durationMs;

    public String getStatus() {
        return status;
    }

    public void setStatus(String status) {
        this.status = status;
    }

    public Map<String, Object> getOutputJson() {
        return outputJson;
    }

    public void setOutputJson(Map<String, Object> outputJson) {
        this.outputJson = outputJson;
    }

    public String getError() {
        return error;
    }

    public void setError(String error) {
        this.error = error;
    }

    public long getDurationMs() {
        return durationMs;
    }

    public void setDurationMs(long durationMs) {
        this.durationMs = durationMs;
    }
}
