export interface ToolCallRequest {
  tenant_id: string;
  agent_id: string;
  tool: string;
  action: string;
  idempotency_key: string;
  params?: Record<string, unknown>;
  resource?: string;
  risk_score?: number;
  risk_factors?: string[];
  user_id?: string;
  session_id?: string;
  trace_id?: string;
  schema_version?: string;
}

export interface ExecutionResult {
  status: string;
  output_json?: Record<string, unknown>;
  error?: string;
  duration_ms: number;
}

export interface ToolCallResponse {
  event_id: string;
  decision: "allow" | "deny" | "approve";
  reason?: string;
  approval_url?: string;
  result?: ExecutionResult;
}

export interface ClientOptions {
  baseUrl: string;
  apiKey: string;
  timeout?: number;
}

export interface WaitForApprovalOptions {
  timeoutMs?: number;
  pollIntervalMs?: number;
}
