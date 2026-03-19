import { randomUUID } from "crypto";
import {
  ClientOptions,
  ToolCallEvent,
  ToolCallRequest,
  ToolCallResponse,
  WaitForApprovalOptions,
} from "./models";
import {
  APIError,
  AuthenticationError,
  OpenClauseError,
  TimeoutError,
} from "./errors";

const MAX_REQUEST_BODY_BYTES = 1_048_576; // 1 MB
const MAX_RESPONSE_BODY_BYTES = 4_194_304; // 4 MB
const DEFAULT_TIMEOUT_MS = 30_000;
const DEFAULT_POLL_INTERVAL_MS = 2_000;
const DEFAULT_WAIT_TIMEOUT_MS = 300_000; // 5 minutes

export class OpenClauseClient {
  private readonly baseUrl: string;
  private readonly apiKey: string;
  private readonly timeoutMs: number;

  constructor(options: ClientOptions) {
    if (!options.baseUrl) throw new OpenClauseError("baseUrl is required");
    if (!options.apiKey) throw new OpenClauseError("apiKey is required");

    this.baseUrl = options.baseUrl.replace(/\/+$/, "");
    this.apiKey = options.apiKey;
    this.timeoutMs = options.timeout ?? DEFAULT_TIMEOUT_MS;
  }

  async submitToolCall(request: ToolCallRequest): Promise<ToolCallResponse> {
    if (request.risk_score !== undefined) {
      if (!Number.isInteger(request.risk_score)) {
        throw new OpenClauseError("risk_score must be an integer");
      }
      if (request.risk_score < 0 || request.risk_score > 10) {
        throw new OpenClauseError("risk_score must be between 0 and 10");
      }
    }
    return this.post<ToolCallResponse>("/v1/toolcalls", request);
  }

  async getEvent(eventId: string): Promise<ToolCallEvent> {
    return this.get<ToolCallEvent>(`/v1/toolcalls/${encodeURIComponent(eventId)}`);
  }

  async execute(eventId: string): Promise<ToolCallResponse> {
    return this.post<ToolCallResponse>(
      `/v1/toolcalls/${encodeURIComponent(eventId)}/execute`,
      {},
    );
  }

  async waitForApproval(
    eventId: string,
    options?: WaitForApprovalOptions,
  ): Promise<ToolCallResponse> {
    const timeoutMs = options?.timeoutMs ?? DEFAULT_WAIT_TIMEOUT_MS;
    const pollIntervalMs = options?.pollIntervalMs ?? DEFAULT_POLL_INTERVAL_MS;
    const deadline = Date.now() + timeoutMs;

    let attempt = 0;
    while (Date.now() < deadline) {
      try {
        // Execute is the source-of-truth: it will return 200 once the grant exists,
        // and return 409 with an "awaiting approval" conflict until then.
        return await this.execute(eventId);
      } catch (err: any) {
        if (!(err instanceof APIError)) throw err;

        const isAwaitingApproval =
          err.statusCode === 409 &&
          (() => {
            const body = err.responseBody;
            if (typeof body === "string") {
              try {
                const parsed = JSON.parse(body);
                const msg = parsed?.message ?? parsed?.error;
                return typeof msg === "string" && msg.toLowerCase().includes("awaiting approval");
              } catch {
                // fallthrough
              }
            }
            return err.message.toLowerCase().includes("awaiting approval");
          })();

        // Retry only when the event is still awaiting approval.
        if (isAwaitingApproval) {
          attempt++;
          const backoff = Math.min(
            pollIntervalMs * Math.pow(2, attempt - 1),
            30_000,
          );
          const remaining = deadline - Date.now();
          if (remaining <= 0) break;
          await this.sleep(Math.min(backoff, remaining));
          continue;
        }

        // Permanent failures: do not retry.
        if (err.statusCode === 400 || err.statusCode === 403 || err.statusCode === 404) {
          throw err;
        }

        throw err;
      }
    }

    throw new TimeoutError(
      `Approval wait timed out after ${timeoutMs}ms for event ${eventId}`,
    );
  }

  static generateIdempotencyKey(): string {
    return randomUUID();
  }

  private async post<T>(path: string, body: unknown): Promise<T> {
    const json = JSON.stringify(body);
    const bodyBytes = new TextEncoder().encode(json);
    if (bodyBytes.length > MAX_REQUEST_BODY_BYTES) {
      throw new OpenClauseError(
        `Request body exceeds ${MAX_REQUEST_BODY_BYTES} byte limit`,
      );
    }

    return this.request<T>(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: json,
    });
  }

  private async get<T>(path: string): Promise<T> {
    return this.request<T>(path, { method: "GET" });
  }

  private async request<T>(
    path: string,
    init: RequestInit,
  ): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);

    try {
      const response = await fetch(url, {
        ...init,
        headers: {
          "X-API-Key": this.apiKey,
          ...init.headers,
        },
        signal: controller.signal,
      });

      if (response.status === 401 || response.status === 403) {
        const body = await response.text();
        throw new AuthenticationError(response.status, body);
      }

      const contentLength = response.headers.get("content-length");
      if (contentLength && parseInt(contentLength, 10) > MAX_RESPONSE_BODY_BYTES) {
        throw new OpenClauseError(
          `Response body exceeds ${MAX_RESPONSE_BODY_BYTES} byte limit`,
        );
      }

      const text = await response.text();

      const actualBytes = new TextEncoder().encode(text).byteLength;
      if (actualBytes > MAX_RESPONSE_BODY_BYTES) {
        throw new OpenClauseError(
          `Response body exceeds ${MAX_RESPONSE_BODY_BYTES} byte limit`,
        );
      }

      if (!response.ok) {
        throw new APIError(
          `API request failed: ${response.status} ${response.statusText}`,
          response.status,
          text,
        );
      }

      return JSON.parse(text) as T;
    } catch (err) {
      if (err instanceof OpenClauseError) throw err;
      if (err instanceof DOMException && err.name === "AbortError") {
        throw new TimeoutError(`Request to ${path} timed out after ${this.timeoutMs}ms`);
      }
      throw new OpenClauseError(
        `Request failed: ${err instanceof Error ? err.message : String(err)}`,
      );
    } finally {
      clearTimeout(timer);
    }
  }

  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
}
