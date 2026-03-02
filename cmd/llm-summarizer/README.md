# LLM Summarizer Service

Minimal FastAPI microservice that generates human-readable summaries for
OpenClause approval notifications.

## Quick Start

```bash
cd cmd/llm-summarizer
pip install -r requirements.txt
uvicorn main:app --reload
```

The service listens on `http://localhost:8000` by default.

## API

### `POST /v1/summarize`

**Request:**

```json
{
  "kind": "approval",
  "payload": {
    "tool": "postgres",
    "action": "query.execute",
    "resource": "prod-db",
    "risk_score": 7,
    "risk_factors": ["production", "write"],
    "reason": "Agent requested write access"
  }
}
```

**Response:**

```json
{
  "summary_text": "Approval requested: postgres.query.execute on prod-db (risk=7, factors=[production, write], reason=Agent requested write access)",
  "model_id": "template",
  "latency_ms": 0,
  "warnings": []
}
```

### `GET /healthz`

Returns `{"status": "ok"}`.

## Configuration

| Variable       | Default                | Description                         |
| -------------- | ---------------------- | ----------------------------------- |
| `USE_HF_MODEL` | `false`                | Set to `true` to load an HF model. |
| `HF_MODEL_ID`  | `google/flan-t5-small` | Hugging Face model identifier.      |
| `PORT`         | `8000`                 | Listen port for uvicorn.            |

## Docker

```bash
docker build -t oc-llm-summarizer .
docker run -p 8000:8000 oc-llm-summarizer
```

To enable the Hugging Face model:

```bash
docker run -p 8000:8000 -e USE_HF_MODEL=true oc-llm-summarizer
```
