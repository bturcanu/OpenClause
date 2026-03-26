# Evidence Bundle Format

OpenClause evidence bundle exports are JSON packages with:

- exported event details for an explicit UTC window
- a manifest with deterministic hashes for the full payload and event list
- event-chain continuity markers (`start_prev_hash`, `end_hash`, `chain_contiguous`)
- an Ed25519 signature over the canonical manifest
- embedded public-key metadata for independent verification

## Manifest fields

```json
{
  "version": "1",
  "generated_at": "2026-03-26T12:00:00Z",
  "event_count": 42,
  "payload_sha256": "…",
  "events_sha256": "…",
  "start_prev_hash": "…",
  "end_hash": "…",
  "chain_contiguous": true,
  "signature_scheme": "ed25519",
  "signing_key_id": "sha256:…",
  "public_key": "base64-ed25519-public-key"
}
```

`manifest_signature` is a base64-encoded Ed25519 signature over the canonical manifest JSON.

## Verification

`cmd/verify` verifies, in order:

1. required bundle fields
2. `event_count`
3. internal event-chain continuity
4. `payload_sha256`
5. `events_sha256`
6. Ed25519 signature validity
7. `signing_key_id` against the actual verification key

By default the verifier uses the bundle's embedded `public_key`. You can override trust with:

- `EVIDENCE_BUNDLE_SIGNING_PUBLIC_KEY`
- `go run ./cmd/verify --bundle ./bundle.json --public-key-file ./openclause-evidence.pub`
- `go run ./cmd/verify --bundle ./bundle.json --public-key "$EVIDENCE_BUNDLE_SIGNING_PUBLIC_KEY"`
- `go run ./cmd/verify --bundle ./bundle.json --require-signing-key-id sha256:...`

For local development, if neither is set, `cmd/verify` can derive the public key from `CONSOLE_JWT_SECRET`.

## Trust model

The embedded public key makes bundles independently verifiable and easy to share between operators. Identity trust still depends on how you distribute or pin the expected public key.

Recommended operator workflow:

1. publish the current verification public key out-of-band
2. pin that key with `--public-key-file` or `EVIDENCE_BUNDLE_SIGNING_PUBLIC_KEY`
3. pin the expected `signing_key_id` during incident review or signer rotation
4. rotate by publishing the next public key first, then updating the signer
