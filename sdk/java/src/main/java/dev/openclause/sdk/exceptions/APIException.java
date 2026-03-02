package dev.openclause.sdk.exceptions;

public class APIException extends OpenClauseException {

    private final int statusCode;
    private final String responseBody;

    public APIException(String message, int statusCode, String responseBody) {
        super(message);
        this.statusCode = statusCode;
        this.responseBody = responseBody;
    }

    public int getStatusCode() {
        return statusCode;
    }

    public String getResponseBody() {
        return responseBody;
    }
}
