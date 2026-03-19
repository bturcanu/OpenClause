export class OpenClauseError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "OpenClauseError";
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

export class APIError extends OpenClauseError {
  public readonly statusCode: number;
  public readonly responseBody?: string;

  constructor(message: string, statusCode: number, responseBody?: string) {
    super(message);
    this.name = "APIError";
    this.statusCode = statusCode;
    this.responseBody = responseBody;
  }
}

export class AuthenticationError extends OpenClauseError {
  public readonly statusCode: number;
  public readonly responseBody?: string;

  constructor(
    statusCode: number,
    responseBody?: string,
    message = "Authentication failed: invalid or missing API key",
  ) {
    super(message);
    this.name = "AuthenticationError";
    this.statusCode = statusCode;
    this.responseBody = responseBody;
  }
}

export class TimeoutError extends OpenClauseError {
  constructor(message = "Request timed out") {
    super(message);
    this.name = "TimeoutError";
  }
}
