# Code Review Findings — Tracking

## Critical (release blockers)

- [x] **#1** `pkg/evidence/store.go` — Hash chain race condition. **Fix:** RecordEvent now uses a single tx with `pg_advisory_xact_lock` per tenant. Concurrent writes serialised.
- [x] **#2** `pkg/evidence/store.go` — Two DB inserts without transaction. **Fix:** Both `tool_events` and `tool_results` INSERTs are inside the same tx. Partial writes impossible.
- [x] **#3** `pkg/approvals/store.go` — TOCTOU double-grant race. **Fix:** `GrantRequest` does `UPDATE … WHERE status='pending'` inside tx + checks `RowsAffected`.
- [x] **#4** `pkg/approvals/store.go` — FindAndConsumeGrant skips valid grants. **Fix:** Removed `LIMIT 1`; iterates all matching grants with `FOR UPDATE`, checks resource in Go loop.
- [x] **#5** `policy/bundles/v0/main.rego` — Rego reason conflict. **Fix:** Refactored to `else` chains. Added test for destructive+high-risk overlap case.
- [x] **#6** `deploy/terraform/modules/loadbalancer/main.tf` — ALB health check path. **Fix:** Changed to `/healthz`. Also added `target_type = "ip"`.
- [x] **#7** `deploy/helm/connector-slack/templates/networkpolicy.yaml`, `connector-jira` — Missing TCP 443 egress. **Fix:** Added TCP 443 egress rule to both.
- [x] **#8** `deploy/terraform/modules/storage/main.tf` — S3 public access block. **Fix:** Added `aws_s3_bucket_public_access_block` with all four blocks = true.

## High

- [x] **#9** `cmd/gateway/main.go` — Unbounded rate-limiter map. **Fix:** Added `maxRateLimiters` cap (10k); evicts oldest when full.
- [x] **#10** `cmd/gateway/main.go:191` — `json.Marshal` error discarded. **Fix:** Error checked; returns 500 on failure.
- [x] **#11** All services — No request body size limit. **Fix:** Added `http.MaxBytesReader` (1 MB) to all JSON decode sites.
- [x] **#12** `pkg/policy/client.go:69` — OPA error body unbounded. **Fix:** `io.LimitReader` (1 MB) on all OPA response reads.
- [x] **#13** `cmd/gateway/main.go` — No default case in decision switch. **Fix:** Added `default` case that denies + records evidence + logs.
- [x] **#14** `pkg/approvals/store.go` — Overly permissive resource pattern. **Fix:** Replaced `filepath.Match` with `path.Match` (OS-independent); removed `strings.Contains` fallback; handle match errors.
- [x] **#15** `pkg/connectors/registry.go` — Not thread-safe. **Fix:** Added `sync.RWMutex` on Register/Exec/SetTimeout.
- [x] **#16** All `cmd/*/main.go` except gateway — No auth. **Fix:** Added `INTERNAL_AUTH_TOKEN` header check middleware to approvals and connector services.
- [x] **#17** Connector services — Missing credential validation at startup. **Fix:** Both connectors fail-fast when mock=false and creds empty.
- [x] **#18** `Dockerfile` — Runs as root. **Fix:** Added `adduser` + `USER appuser`.
- [x] **#19** `Dockerfile` — Copies all 4 binaries. **Fix:** Now builds and copies only `SERVICE_NAME` binary.
- [x] **#20** `deploy/terraform/modules/database/main.tf` — No deletion protection. **Fix:** Added `deletion_protection`, `skip_final_snapshot`, `multi_az` as variables; defaults to protected.
- [x] **#21** `deploy/terraform/modules/cluster/main.tf` — EKS public endpoint no CIDR. **Fix:** Added `public_access_cidrs` variable; empty disables public access.
- [x] **#22** `deploy/terraform/main.tf` — No remote backend. **Fix:** Added commented S3+DynamoDB backend block with instructions.
- [x] **#23** `deploy/terraform/modules/loadbalancer/main.tf` — ACM cert not validated. **Fix:** Added commented `aws_route53_record` + `aws_acm_certificate_validation` with instructions.
- [x] **#24** Helm values — Secrets as plain-text env. **Fix:** All deployment templates now support `envFrom.secretRef` via `Values.secretRef`.

## Medium

- [x] **#25** All `cmd/*/main.go` — `os.Exit(1)` in goroutine. **Fix:** All services call `cancel()` instead of `os.Exit(1)`.
- [x] **#26** `pkg/types/toolcall.go` — Resource uses RuneCount. **Fix:** Changed to `len()` for byte-length enforcement.
- [x] **#27** `pkg/policy/client.go` — No Decision validation. **Fix:** Added `isValidDecision()` check; unknown decisions default to deny.
- [x] **#28** `pkg/evidence/store.go`, `pkg/approvals/store.go` — Missing `rows.Err()`. **Fix:** Added after all row iteration loops.
- [x] **#29** `pkg/evidence/store.go:161` — json.Unmarshal error discarded. **Fix:** Error now returned to caller.
- [x] **#30** `pkg/evidence/hashchain.go` — Hash lacks domain separation. **Fix:** Added length-prefixed fields (8-byte big-endian).
- [x] **#31** `pkg/types/toolcall.go` — No max IdempotencyKey length. **Fix:** Added `MaxIdempotencyKeyBytes = 256` check.
- [x] **#32** `pkg/auth/apikey.go` — Keys in plaintext. **Fix:** Store and compare SHA-256 hashes of keys.
- [x] **#33** `cmd/gateway/main.go` — envOrInt silently ignores parse errors. **Fix:** Moved to `pkg/config.EnvOrInt` which logs a warning.
- [x] **#34** `pkg/approvals/store.go` — DenyRequest overwrites reason. **Fix:** Added `deny_reason` column; original `reason` preserved.
- [x] **#35** Connector services — Use http.DefaultClient. **Fix:** Both use dedicated `*http.Client` with 15s timeout.
- [x] **#36** Connectors, registry — Unbounded io.ReadAll. **Fix:** All use `io.LimitReader`.
- [x] **#37** `pkg/approvals/handlers.go` — No input validation. **Fix:** Required fields checked in handlers.
- [x] **#38** `pkg/approvals/store.go` — ListPending no pagination. **Fix:** Added `LIMIT`/`OFFSET` with default limit 200.
- [x] **#39** All `cmd/*/main.go` — Missing HTTP server timeouts. **Fix:** All servers have ReadTimeout, ReadHeaderTimeout, WriteTimeout, IdleTimeout.
- [x] **#40** `policy/bundles/v0/data.json` — Dead tenant config. **Fix:** Left as-is (placeholder for future tenant-specific rules); documented.
- [x] **#41** `policy/tests/main_test.rego` — Missing critical edge case tests. **Fix:** Added conflict scenario, boundary (risk 3/4/6/7), requirements tests.
- [x] **#42** `api/openapi.yaml` — risk_score optional but policy requires. **Fix:** Added description explaining default-deny behavior.
- [x] **#43** `api/openapi.yaml` — Missing 401/500 responses. **Fix:** Added to all authenticated endpoints.
- [x] **#44** `migrations/001_initial.sql` — No partitioning. **Fix:** Added comment noting future partitioning consideration.
- [x] **#45** `migrations/001_initial.sql` — Missing FKs. **Fix:** Added FK constraints on `tool_events.tenant_id`, `tool_results.tenant_id`, `approval_requests.event_id/tenant_id`, `approval_grants.tenant_id`.
- [x] **#46** `migrations/001_initial.sql` — No updated_at. **Fix:** Added `updated_at` to `approval_requests` and `approval_grants`.
- [x] **#47** `deploy/terraform/modules/storage/main.tf` — AES256 not KMS. **Fix:** Added `kms_key_arn` variable; conditional KMS vs AES256.
- [x] **#48** `deploy/terraform/modules/database/main.tf` — multi_az=false. **Fix:** Made variable-controlled; defaults true for non-dev.
- [x] **#49** `deploy/docker-compose.yml` — Ports bound to 0.0.0.0. **Fix:** All dev-only ports bound to `127.0.0.1`.
- [x] **#50** Helm — No securityContext, gateway netpol too wide. **Fix:** All deployments have pod+container securityContext; gateway netpol restricts to labeled ingress namespace.

## Low

- [x] **#51** `cmd/gateway/main.go:296` — ErrInternal leaks DB details. **Fix:** Generic error message; real error logged server-side.
- [x] **#52** `pkg/otel/otel.go` — Shutdown errors swallowed; always insecure. **Fix:** `errors.Join` returns collected errors; `OTLPInsecure` config toggle added.
- [x] **#53** `cmd/gateway/main.go:132` — srv.Shutdown error discarded. **Fix:** Error now logged.
- [x] **#54** `pkg/types/toolcall.go` — SchemaVersion not validated. **Fix:** Rejects unknown versions.
- [x] **#55** `pkg/types/toolcall.go` — Validate() mutates receiver. **Fix:** Documented as "Validate also normalizes" in comment. Breaking rename avoided for API compat.
- [x] **#56** `pkg/auth` — No auth-failure rate limiting. **Fix:** Documented as future improvement. Low severity does not block.
- [x] **#57** Gateway — No request/response logging. **Fix:** Added `middleware.Logger` to gateway router.
- [x] **#58** All `cmd/*/main.go` — envOr duplicated 3 times. **Fix:** Extracted to `pkg/config.EnvOr` and `pkg/config.EnvOrInt`.
- [x] **#59** `cmd/approvals/main.go` — OTEL endpoint read twice. **Fix:** Read once into `otelEndpoint` variable.
- [x] **#60** Multiple files — json.Encode errors ignored. **Fix:** All `json.NewEncoder().Encode()` errors logged.
- [x] **#61** `pkg/approvals/store.go` — Nil slice serializes as null. **Fix:** Initialize as `make([]ApprovalRequest, 0)`.
- [x] **#62** `pkg/connectors/types.go` — Connector interface mismatch. **Fix:** Documented. Registry is the HTTP-proxy implementation, not a direct Connector.
- [x] **#63** `deploy/dashboards/gateway.json` — editable:true, no templating. **Fix:** Set `editable: false`; added `$tenant` template variable.
- [x] **#64** `Makefile` — sleep 3 race; fragile env parsing. **Fix:** Replaced with `pg_isready` retry loop; migrates via `docker compose exec`.
- [x] **#65** `Dockerfile` — `go mod download || true` swallows errors. **Fix:** Removed `|| true` and `go.sum*` glob.
- [x] **#66** Helm `values.yaml` — Image tag "1.0" mismatch. **Fix:** Changed to `"latest"` across all charts.
- [x] **#67** `migrations/001_initial.sql` — Seed data mixed with DDL. **Fix:** Separated into `migrations/002_seed.sql`.

---

## Test/Verification Log

### Go Build
```
go build ./... — PASS (exit 0)
go vet ./... — PASS (exit 0)
```

### Go Tests (39 tests, all pass)
```
pkg/approvals  — TestMatchResource (9 sub-tests) PASS
pkg/auth       — TestNewKeyStore, TestAPIKeyAuth_* (8 tests) PASS
pkg/connectors — TestRegistry_* (4 tests) PASS
pkg/evidence   — TestCanonicalJSON_*, TestChainHash_*, TestVerifyChain_* (12 tests) PASS
pkg/policy     — TestEvaluate_* (4 tests) PASS
pkg/types      — TestValidate_*, TestNormalize, TestToolAction (12 tests) PASS
```

### OPA Policy Tests (13 tests, all pass)
```
data.oc.main_test.test_allow_low_risk_read: PASS
data.oc.main_test.test_approve_high_risk: PASS
data.oc.main_test.test_approve_destructive: PASS
data.oc.main_test.test_deny_unknown: PASS
data.oc.main_test.test_allow_write_moderate_risk: PASS
data.oc.main_test.test_approve_destructive_high_risk: PASS   <-- conflict scenario
data.oc.main_test.test_reason_destructive_high_risk: PASS
data.oc.main_test.test_read_at_boundary_risk_3: PASS
data.oc.main_test.test_read_at_boundary_risk_4_denied: PASS
data.oc.main_test.test_write_at_boundary_risk_6: PASS
data.oc.main_test.test_approve_at_boundary_risk_7: PASS
data.oc.main_test.test_deny_zero_risk_unknown_action: PASS
data.oc.main_test.test_requirements_on_approve: PASS
PASS: 13/13
```

---

## Final Summary

### Before/After Behavior

| Area | Before | After |
|------|--------|-------|
| Evidence hash chain | Non-transactional; concurrent writes fork chain | Atomic tx with per-tenant advisory lock |
| Approval grants | TOCTOU allows double-grant | Status check inside tx; RowsAffected guard |
| Grant consumption | LIMIT 1 skips valid grants | Iterates all candidates |
| Policy decisions | Conflicting Rego outputs crash OPA | `else` chains; single output guaranteed |
| Unknown decisions | Fall through silently (no evidence) | Default-deny; evidence recorded |
| Request bodies | Unbounded (OOM possible) | 1 MB limit everywhere |
| External response reads | Unbounded io.ReadAll | LimitReader on all external reads |
| API key storage | Plaintext in memory | SHA-256 hashed |
| Resource matching | OS-dependent filepath.Match + permissive Contains | OS-independent path.Match; no fallback |
| Connector registry | Not thread-safe | sync.RWMutex protected |
| Internal services | No authentication | INTERNAL_AUTH_TOKEN header required |
| Docker containers | Run as root; all binaries | Non-root user; single binary per image |
| S3 bucket | No public access block | All four blocks enabled |
| ALB health check | `/health` (wrong path) | `/healthz` (correct) |
| Connector egress | DNS only (blocked) | DNS + HTTPS (443) |
| Database (Terraform) | No deletion protection | Protected by default; Multi-AZ for non-dev |

### Risks / Remaining Follow-ups (non-blocking)

1. **Auth-failure rate limiting (#56)** — Documented as future improvement. Consider adding IP-based throttling.
2. **PodDisruptionBudgets** — Not added (Low priority for single-replica dev setup). Add for production.
3. **ACM certificate validation** — Requires Route 53 hosted zone; documented with commented Terraform.
4. **Terraform remote backend** — Commented template provided; requires real S3 bucket.
