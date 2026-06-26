# API

## Authentication

All endpoints except `GET /health`, `GET /health/ready`, `GET /swagger`, preview/keep webhook links require:

```
Authorization: Bearer YOUR_API_KEY
```

The API key is stored in `data/.credentials` and printed once on first startup. The same key authenticates `POST /internal/ingest` (Postfix reads it from the shared `data/` volume).

## Example requests

Run from a container on the Docker network (Postfix image includes curl):

From the host (API mapped to `PORT` in `.env`, default `8080`):

```bash
curl -s http://localhost:8080/health
curl -s http://localhost:8080/health/ready
curl -s -H "Authorization: Bearer YOUR_API_KEY" "http://localhost:8080/emails?limit=10"
curl -s -H "Authorization: Bearer YOUR_API_KEY" "http://localhost:8080/emails/stats"
curl -s -H "Authorization: Bearer YOUR_API_KEY" "http://localhost:8080/emails/latest"
curl -s -H "Authorization: Bearer YOUR_API_KEY" "http://localhost:8080/emails/latest?limit=5"
curl -s -H "Authorization: Bearer YOUR_API_KEY" "http://localhost:8080/emails/wait?to=user@example.com&since_id=0&timeout=25"
curl -s -H "Authorization: Bearer YOUR_API_KEY" "http://localhost:8080/emails/1?track_open=false"
curl -s -H "Authorization: Bearer YOUR_API_KEY" "http://localhost:8080/emails/1/raw"
```

From the Docker network:

```bash
# Health (no auth)
docker compose exec postfix sh -c 'curl -s http://api:${PORT:-8080}/health'

# List emails
docker compose exec postfix sh -c 'curl -s \
  -H "Authorization: Bearer YOUR_API_KEY" \
  "http://api:${PORT:-8080}/emails?limit=10"'

# Get email by ID
docker compose exec postfix sh -c 'curl -s \
  -H "Authorization: Bearer YOUR_API_KEY" \
  http://api:${PORT:-8080}/emails/1'

# Delete email
docker compose exec postfix sh -c 'curl -s -X DELETE \
  -H "Authorization: Bearer YOUR_API_KEY" \
  http://api:${PORT:-8080}/emails/1'
```

Makefile shortcut:

```bash
make api-health
```

### Query parameters (`GET /emails`, `DELETE /emails`, `GET /emails/wait`)

| Param | Description |
|-------|-------------|
| `from` | Filter by sender (substring) |
| `to` | Filter by recipient (substring) |
| `subject` | Filter by subject (substring) |
| `message_id` | Exact match on SMTP Message-ID |
| `limit` | Max results (default 50, max 200) |
| `offset` | Pagination offset |
| `after` | RFC3339 or `YYYY-MM-DD` |
| `before` | RFC3339 or `YYYY-MM-DD` |
| `kept` | `true` or `false` — retention keep flag |
| `opened` | `true` or `false` — whether `opened_at` is set |

`GET /emails/wait` also accepts `since_id` (only emails with id greater than this) and `timeout` seconds (default 25, capped below server request timeout). Pass `since_id` of the current newest email to block until new mail arrives.

`GET /emails/{id}` and `GET /emails/{id}/raw` accept `track_open=false` to read without setting `opened_at`.

Results are ordered newest first.

## Swagger

OpenAPI 3 spec and UI (internal network):

- `http://api:8080/swagger`
- `http://api:8080/swagger/openapi.yaml`
