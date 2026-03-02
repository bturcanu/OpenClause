package dev.openclause.sdk.exceptions;

public class OpenClauseException extends Exception {

    public OpenClauseException(String message) {
        super(message);
    }

    public OpenClauseException(String message, Throwable cause) {
        super(message, cause);
    }
}
