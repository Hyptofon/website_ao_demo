# University Chatbot API Documentation

> Version 1.0.0 — TZ §10.3 Compliance

## Base URL

```
Production: https://<backend>.railway.app
Development: http://localhost:8080
```

## Authentication

All admin endpoints require one of:
- **JWT Bearer Token**: `Authorization: Bearer <jwt>`
- **Admin Token**: `X-Admin-Token: <token>` (legacy)

Admin endpoints are mounted under `/admin-{hash}` where `{hash}` is derived from `ADMIN_TOKEN` via SHA-256.

---

## Public Endpoints

### Chat

#### `GET /api/chat/stream`

Stream a RAG-powered chatbot response via Server-Sent Events (SSE).

**Query Parameters:**
| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `message` | string | ✅ | User question (max 500 chars) |
| `session_id` | string | ✅ | Browser session identifier |
| `language` | string | ✅ | `uk` or `en` |

**Response**: `text/event-stream`
```
event: token
data: <text chunk>

event: sources
data: [{"document_name":"...", "score":0.95, "page_number":1}]

event: meta
data: {"query_hash":"a1b2c3d4", "variant_id":0}

event: done
data: [DONE]
```

**Error Responses:**
- `429 Too Many Requests` — Rate limited. Includes `Retry-After` header.
- `400 Bad Request` — Validation error (empty/too long message, XSS detected).

---

#### `POST /api/chat/feedback`

Submit 👍/👎 feedback for a response.

**Body (JSON):**
```json
{
  "query_hash": "a1b2c3d4e5f6a7b8",
  "feedback": 1
}
```
`feedback`: `1` (positive) or `-1` (negative).

---

#### `GET /api/chat/suggestions`

Get suggested questions.

**Query Parameters:**
| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `language` | string | `uk` | `uk` or `en` |
| `limit` | int | `5` | Max suggestions |

---

### Health

#### `GET /healthz`
Returns `200 OK` with `{"status":"ok"}`.

---

## Admin Auth Endpoints (Public)

All prefixed with `/admin-{hash}`.

#### `GET /auth/login`
Redirects to Google OAuth consent screen.

#### `GET /auth/callback`
OAuth callback. Sets `refresh_token` httpOnly cookie, redirects to frontend with `#token=<jwt>`.

#### `POST /auth/refresh`
Exchange refresh token (from cookie) for new access token.

**Response:**
```json
{"token": "<new_jwt>", "expires_in": "86400"}
```

---

## Admin Protected Endpoints

All prefixed with `/admin-{hash}`. Require authentication.

### Documents

#### `GET /documents`
List all indexed documents.

#### `POST /documents/upload`
Upload and index a document (async). Supported: `.pdf`, `.docx`, `.xlsx`, `.txt`.

**Body**: `multipart/form-data` with field `file`.

**Response** `202 Accepted`:
```json
{"status": "accepted", "job_id": "a1b2c3d4"}
```

#### `GET /documents/jobs/{job_id}`
Check indexing job status.

#### `DELETE /documents/{document_id}`
Delete document and all its vector chunks.

#### `POST /documents/{document_id}/reindex`
Re-index an existing document. `202 Accepted`.

#### `PUT /documents/{id}/rename`
Rename a document.
```json
{"filename": "new_name.pdf"}
```

#### `GET /documents/{id}/download`
Download the raw document file.

### Analytics

#### `GET /analytics/summary?days=30`
Aggregated stats.
```json
{
  "total_queries": 1500,
  "blocked_queries": 23,
  "positive_feedback": 120,
  "negative_feedback": 15,
  "avg_response_ms": 850.5
}
```

#### `GET /analytics/daily?days=30`
Per-day stats array.

#### `GET /analytics/top-queries?days=30&limit=10`
Most frequent queries.

#### `GET /analytics/feedback?days=30`
Feedback ratio.
```json
{"total": 135, "positive": 120, "negative": 15, "ratio": 0.888}
```

#### `GET /analytics/export/csv?days=30`
Export analytics as CSV file.

### Queries

#### `GET /queries?days=30&limit=50`
Individual query rows for admin inspection.

### Audit Log

#### `GET /audit?offset=0&limit=20`
Admin action log.
```json
{
  "entries": [{"id":1, "admin_email":"...", "action":"login", "target":"oauth", "created_at":"..."}],
  "total": 42
}
```

### Prompts (A/B Testing)

#### `GET /prompts`
List all prompt variants.

#### `POST /prompts`
Create a new variant.
```json
{"name": "concise_v2", "language": "uk", "prompt_text": "...", "is_active": true}
```

#### `PUT /prompts/{id}`
Update prompt text.

#### `PATCH /prompts/{id}/toggle`
Toggle active status.

#### `DELETE /prompts/{id}`
Delete a variant.

### Suggestions

#### `POST /suggestions`
Add manual suggestion.

---

## Rate Limiting

- **Window**: 5 minutes
- **Limit**: 10 requests per window (configurable via `RATE_LIMIT_PER_MIN`)
- **Penalty**: ×3 weight for off-topic/blocked queries
- **Response**: `429` with `Retry-After` header and JSON body

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GEMINI_API_KEY` | ✅ | — | Google Gemini API key |
| `QDRANT_URL` | ✅ | `localhost` | Qdrant endpoint |
| `QDRANT_API_KEY` | — | — | Qdrant API key |
| `DB_PATH` | — | `./data/analytics.db` | SQLite path |
| `PORT` | — | `8080` | Server port |
| `ALLOWED_ORIGINS` | — | `localhost:4321,localhost:3000` | CORS origins |
| `ADMIN_TOKEN` | — | — | Legacy admin auth token |
| `RATE_LIMIT_PER_MIN` | — | `10` | Rate limit per 5-min window |
| `GOOGLE_CLIENT_ID` | — | — | OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | — | — | OAuth client secret |
| `OAUTH_REDIRECT_URL` | — | `localhost:8080/admin/auth/callback` | OAuth redirect |
| `JWT_SECRET` | — | (insecure default) | JWT signing secret |
| `ADMIN_ALLOWED_EMAILS` | — | — | Comma-separated emails or `@domain` patterns |
| `UPSTASH_REDIS_REST_URL` | — | — | Upstash Redis URL |
| `UPSTASH_REDIS_REST_TOKEN` | — | — | Upstash Redis token |
| `ENABLE_RERANKING` | — | `false` | Enable LLM-based reranking |
| `FRONTEND_URL` | — | `localhost:4321/admin` | Admin panel URL |
