"""LLM Summarizer microservice for OpenClause.

Provides human-readable summaries for approval notifications. Runs as a
standalone FastAPI service called by the Go gateway's LLMSummaryProvider.

Environment variables:
    USE_HF_MODEL  — set to "true" to load a Hugging Face model for inference.
    HF_MODEL_ID   — model identifier on the Hub (default: google/flan-t5-small).
    PORT           — listen port (default: 8000).
"""

from __future__ import annotations

import hashlib
import json
import os
import time
from typing import Any

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

app = FastAPI(title="OpenClause LLM Summarizer", version="0.1.0")

# ── In-memory cache (hash of input → response) ─────────────────────────────

_cache: dict[str, dict[str, Any]] = {}

# ── Optional HF model ──────────────────────────────────────────────────────

_hf_pipeline: Any | None = None
_hf_model_id: str = os.getenv("HF_MODEL_ID", "google/flan-t5-small")

if os.getenv("USE_HF_MODEL", "").lower() == "true":
    try:
        from transformers import pipeline as hf_pipeline  # type: ignore[import-untyped]

        _hf_pipeline = hf_pipeline("text2text-generation", model=_hf_model_id)
    except Exception as exc:
        import logging

        logging.warning("Failed to load HF model %s: %s", _hf_model_id, exc)


# ── Models ──────────────────────────────────────────────────────────────────


class SummarizePayload(BaseModel):
    tool: str = ""
    action: str = ""
    resource: str = ""
    risk_score: int = 0
    risk_factors: list[str] = []
    reason: str = ""
    tenant_id: str = ""
    agent_id: str = ""


class SummarizeRequest(BaseModel):
    kind: str
    payload: SummarizePayload


class SummarizeResponse(BaseModel):
    summary_text: str
    model_id: str
    latency_ms: int
    warnings: list[str] = []


# ── Helpers ─────────────────────────────────────────────────────────────────


def _cache_key(req: SummarizeRequest) -> str:
    raw = json.dumps(req.model_dump(), sort_keys=True)
    return hashlib.sha256(raw.encode()).hexdigest()


def _template_summary(p: SummarizePayload) -> str:
    factors = ", ".join(p.risk_factors) if p.risk_factors else "none"
    return (
        f"Approval requested: {p.tool}.{p.action} on {p.resource} "
        f"(risk={p.risk_score}, factors=[{factors}], reason={p.reason})"
    )


def _hf_summary(p: SummarizePayload) -> str:
    prompt = (
        f"Summarize this approval request in one sentence: "
        f"tool={p.tool}, action={p.action}, resource={p.resource}, "
        f"risk_score={p.risk_score}, reason={p.reason}"
    )
    assert _hf_pipeline is not None
    result = _hf_pipeline(prompt, max_new_tokens=120)
    return str(result[0]["generated_text"]).strip()


# ── Endpoint ────────────────────────────────────────────────────────────────


@app.post("/v1/summarize", response_model=SummarizeResponse)
def summarize(req: SummarizeRequest) -> SummarizeResponse:
    if req.kind != "approval":
        raise HTTPException(status_code=400, detail=f"unsupported kind: {req.kind}")

    key = _cache_key(req)
    if key in _cache:
        cached = _cache[key]
        return SummarizeResponse(**cached)

    start = time.monotonic()
    warnings: list[str] = []

    if _hf_pipeline is not None:
        try:
            text = _hf_summary(req.payload)
            model_id = f"hf-{_hf_model_id}"
        except Exception as exc:
            warnings.append(f"HF inference failed: {exc}")
            text = _template_summary(req.payload)
            model_id = "template"
    else:
        text = _template_summary(req.payload)
        model_id = "template"

    latency_ms = int((time.monotonic() - start) * 1000)

    resp = SummarizeResponse(
        summary_text=text,
        model_id=model_id,
        latency_ms=latency_ms,
        warnings=warnings,
    )

    _cache[key] = resp.model_dump()
    return resp


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok"}
