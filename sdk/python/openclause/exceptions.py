"""OpenClause SDK exceptions."""


class OpenClauseError(Exception):
    """Base exception for all OpenClause SDK errors."""


class APIError(OpenClauseError):
    """Raised when the API returns a non-success HTTP status code."""

    def __init__(self, status_code: int, message: str) -> None:
        self.status_code = status_code
        self.message = message
        super().__init__(f"HTTP {status_code}: {message}")


class AuthenticationError(OpenClauseError):
    """Raised when API authentication fails (401/403)."""


class TimeoutError(OpenClauseError):  # noqa: A001
    """Raised when a request or polling operation times out."""


class ValidationError(OpenClauseError):
    """Raised when a request fails local validation before being sent."""
