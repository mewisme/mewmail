# MewMailAPI

Lightweight self-hosted email receiving service. Postfix accepts inbound SMTP for a catch-all domain and pipes messages to an internal Go API that stores them in SQLite. Query mail over a Bearer-authenticated REST API.

**Receive only** — this project does not send email.

## Architecture

```
Internet SMTP :25 → Postfix → ingest-mail → POST /internal/ingest → Go API → SQLite
```

- **Public ports:** `25` (SMTP) and `PORT` (API, default `8080`) — both from `.env`
- **Internal auth:** Postfix reads `api_key` from `data/.credentials` (same key as the REST API)

## Requirements

- Docker and Docker Compose
- ARM64 or AMD64 Linux VPS (tested target: 512 MB RAM)
- A domain with DNS control

## Quick start

```bash
cp .env.example .env
# Edit .env: set DOMAIN
mkdir -p data && chmod 700 data
docker compose pull
docker compose up -d
docker compose logs api   # capture API key on first startup
```

For local builds:

```bash
docker compose up -d --build
```

On first startup the API generates an API key and prints it **once** to the API container logs. It is also stored in `data/.credentials` (mode `0600`).

## DNS configuration

### Mail (required)

| Type | Name | Value |
|------|------|-------|
| MX | `@` | `10 mail.example.com` |
| A | `mail` | `<VPS public IP>` |

Replace `example.com` with your `DOMAIN`. All addresses `*@DOMAIN` are accepted (catch-all).

### API

The API is exposed on the host at `PORT` (default `8080`). Restrict access via firewall if needed.

## Firewall

Allow inbound **TCP 25** (SMTP). Open **TCP `PORT`** only if you need direct API access on the host.

## API authentication

All endpoints except `GET /health` and `GET /swagger` require:

```
Authorization: Bearer YOUR_API_KEY
```

The API key is stored in `data/.credentials` and printed once on first startup. The same key authenticates `POST /internal/ingest` (Postfix reads it from the shared `data/` volume).

## Example requests

Run from a container on the Docker network (Postfix image includes curl):

From the host (API mapped to `PORT` in `.env`, default `8080`):

```bash
curl -s http://localhost:8080/health
curl -s -H "Authorization: Bearer YOUR_API_KEY" "http://localhost:8080/emails?limit=10"
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

### Query parameters (`GET /emails`, `DELETE /emails`)

| Param | Description |
|-------|-------------|
| `from` | Filter by sender (substring) |
| `to` | Filter by recipient (substring) |
| `subject` | Filter by subject (substring) |
| `limit` | Max results (default 50, max 200) |
| `offset` | Pagination offset |
| `after` | RFC3339 or `YYYY-MM-DD` |
| `before` | RFC3339 or `YYYY-MM-DD` |

Results are ordered newest first.

## Swagger

OpenAPI 3 spec and UI (internal network):

- `http://api:8080/swagger`
- `http://api:8080/swagger/openapi.yaml`

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DOMAIN` | — | Mail domain for Postfix (catch-all) |
| `PORT` | `8080` | API listen port (host + container). Changing this requires `docker compose up -d --force-recreate` so Postfix picks up the new port (written to `/var/spool/postfix/.ingest-port` at startup). |
| `EMAIL_RETENTION_DAYS` | `7` | Auto-delete after N days |
| `ALLOW_MULTIPART` | `false` | Accept multipart MIME messages |
| `WEBHOOK_URL` | — | Optional HTTP webhook for events (Discord-compatible) |

`API_HOST`, `DB_PATH` — optional overrides for the API (defaults: `0.0.0.0`, `/data/mail.db`). Not needed for Docker deploys.

### Webhooks

Set `WEBHOOK_URL` to receive notifications when emails are received or auto-cleaned.

- **Discord**: paste a channel webhook URL (`https://discord.com/api/webhooks/...`). Messages appear as embeds from **MewMail**.
- **Other endpoints**: receive JSON `{"event":"email.received"|"email.cleaned","timestamp":"...","data":{...}}`.

Delivery is fire-and-forget; failures are logged and do not affect ingest or cleanup.

## Container images (GHCR)

Images are built and pushed to GitHub Container Registry on push to `main`/`master`, version tags (`v*`), or manual workflow dispatch.

Published as:

- `ghcr.io/mewisme/mewmail/api:latest`
- `ghcr.io/mewisme/mewmail/postfix:latest`

Image tags are set in `docker-compose.yml`.

For private packages, log in on the VPS:

```bash
echo "$GITHUB_TOKEN" | docker login ghcr.io -u USERNAME --password-stdin
docker compose pull
```

A background cleaner runs hourly and deletes emails older than `EMAIL_RETENTION_DAYS`. Attachment metadata is removed via foreign-key cascade. `VACUUM` runs approximately daily.

## Database backup

```bash
make backup-db
# or copy the file while containers are stopped:
cp data/mail.db backups/mail-$(date +%Y%m%d).db
```

## Database restore

```bash
docker compose down
cp backups/mail-YYYYMMDD.db data/mail.db
chmod 600 data/mail.db
docker compose up -d
```

## Updating

```bash
git pull
docker compose pull
docker compose up -d
```

Data in `./data` persists across rebuilds.

## Development

```bash
cd api
go test ./...
go run ./cmd/server
```

## Security notes

- Rotate the API key by regenerating `data/.credentials` (stop stack, delete file, restart — new key printed once)
- API key is in `data/.credentials` — back it up securely
- Never log or commit secrets
- Prepared SQL statements throughout; request body limited to 10 MiB

## License

MIT — see [LICENSE](LICENSE).
