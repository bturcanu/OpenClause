package dev.openclause.sdk;

import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import dev.openclause.sdk.exceptions.APIException;
import dev.openclause.sdk.exceptions.OpenClauseException;
import dev.openclause.sdk.models.ToolCallRequest;
import dev.openclause.sdk.models.ToolCallResponse;

import java.io.IOException;
import java.net.URI;
import java.net.URLEncoder;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.UUID;

public class OpenClauseClient {

    private static final int MAX_REQUEST_BODY_BYTES = 1_048_576;  // 1 MB
    private static final int MAX_RESPONSE_BODY_BYTES = 4_194_304; // 4 MB
    private static final int DEFAULT_TIMEOUT_SECONDS = 30;
    private static final long DEFAULT_WAIT_TIMEOUT_MS = 300_000;  // 5 minutes
    private static final long DEFAULT_POLL_INTERVAL_MS = 2_000;

    private final String baseUrl;
    private final String apiKey;
    private final HttpClient httpClient;
    private final Gson gson;

    public OpenClauseClient(String baseUrl, String apiKey) {
        this(baseUrl, apiKey, DEFAULT_TIMEOUT_SECONDS);
    }

    public OpenClauseClient(String baseUrl, String apiKey, int timeoutSeconds) {
        if (baseUrl == null || baseUrl.isEmpty()) {
            throw new IllegalArgumentException("baseUrl is required");
        }
        if (apiKey == null || apiKey.isEmpty()) {
            throw new IllegalArgumentException("apiKey is required");
        }
        this.baseUrl = baseUrl.replaceAll("/+$", "");
        this.apiKey = apiKey;
        this.httpClient = HttpClient.newBuilder()
                .connectTimeout(Duration.ofSeconds(timeoutSeconds))
                .build();
        this.gson = new GsonBuilder().create();
    }

    public ToolCallResponse submitToolCall(ToolCallRequest request) throws OpenClauseException {
        String json = gson.toJson(request);
        return post("/v1/tool-calls", json, ToolCallResponse.class);
    }

    public ToolCallResponse getEvent(String eventId) throws OpenClauseException {
        String encoded = URLEncoder.encode(eventId, StandardCharsets.UTF_8);
        return get("/v1/tool-calls/" + encoded, ToolCallResponse.class);
    }

    public ToolCallResponse execute(String eventId) throws OpenClauseException {
        String encoded = URLEncoder.encode(eventId, StandardCharsets.UTF_8);
        return post("/v1/tool-calls/" + encoded + "/execute", "{}", ToolCallResponse.class);
    }

    public ToolCallResponse waitForApproval(String eventId, long timeoutMs, long pollIntervalMs) throws OpenClauseException {
        long deadline = System.currentTimeMillis() + timeoutMs;
        int attempt = 0;

        while (System.currentTimeMillis() < deadline) {
            ToolCallResponse response = getEvent(eventId);
            if (!"approve".equals(response.getDecision())) {
                return response;
            }

            attempt++;
            long backoff = Math.min(
                    pollIntervalMs * (long) Math.pow(2, attempt - 1),
                    30_000
            );
            long remaining = deadline - System.currentTimeMillis();
            if (remaining <= 0) break;

            try {
                Thread.sleep(Math.min(backoff, remaining));
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                throw new OpenClauseException("Wait interrupted", e);
            }
        }

        throw new OpenClauseException(
                "Approval wait timed out after " + timeoutMs + "ms for event " + eventId
        );
    }

    public static String generateIdempotencyKey() {
        return UUID.randomUUID().toString();
    }

    private <T> T post(String path, String jsonBody, Class<T> responseType) throws OpenClauseException {
        byte[] bodyBytes = jsonBody.getBytes(StandardCharsets.UTF_8);
        if (bodyBytes.length > MAX_REQUEST_BODY_BYTES) {
            throw new OpenClauseException("Request body exceeds " + MAX_REQUEST_BODY_BYTES + " byte limit");
        }

        HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(baseUrl + path))
                .header("X-API-Key", apiKey)
                .header("Content-Type", "application/json")
                .POST(HttpRequest.BodyPublishers.ofString(jsonBody))
                .build();

        return send(request, responseType);
    }

    private <T> T get(String path, Class<T> responseType) throws OpenClauseException {
        HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(baseUrl + path))
                .header("X-API-Key", apiKey)
                .GET()
                .build();

        return send(request, responseType);
    }

    private <T> T send(HttpRequest request, Class<T> responseType) throws OpenClauseException {
        try {
            HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());

            int statusCode = response.statusCode();
            String body = response.body();

            if (body != null && body.getBytes(StandardCharsets.UTF_8).length > MAX_RESPONSE_BODY_BYTES) {
                throw new OpenClauseException("Response body exceeds " + MAX_RESPONSE_BODY_BYTES + " byte limit");
            }

            if (statusCode == 401 || statusCode == 403) {
                throw new APIException("Authentication failed: invalid or missing API key", statusCode, body);
            }

            if (statusCode < 200 || statusCode >= 300) {
                throw new APIException(
                        "API request failed: " + statusCode,
                        statusCode,
                        body
                );
            }

            return gson.fromJson(body, responseType);
        } catch (IOException e) {
            throw new OpenClauseException("Request failed: " + e.getMessage(), e);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            throw new OpenClauseException("Request interrupted", e);
        }
    }
}
