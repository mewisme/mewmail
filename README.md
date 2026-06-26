# MewMailAPI

Lightweight self-hosted email receiving service. Postfix accepts inbound SMTP for a catch-all domain and pipes messages to an internal Go API that stores them in SQLite. Query mail over a Bearer-authenticated REST API.

**Receive only** — this project does not send email.

## Architecture

```
Internet SMTP :25 → Postfix → ingest-mail → POST /api/internal/ingest → Go API → SQLite
```

- **Public ports:** `25` (SMTP) and `PORT` (API, default `8080`) — both from `.env`
- **Internal auth:** Postfix reads `internal_key` from `data/.credentials` (separate from the external REST `api_key`)

## Quick start

```bash
cp .env.example .env
# Edit .env: set DOMAIN
mkdir -p data && chmod 700 data
docker compose pull
docker compose up -d
docker compose logs api   # capture API keys on first startup
```

See [docs/deployment.md](docs/deployment.md) for DNS, firewall, and updates.

## Documentation

| Topic | Guide |
|-------|-------|
| Deploy, DNS, firewall, updates | [docs/deployment.md](docs/deployment.md) |
| API auth, examples, Swagger | [docs/api.md](docs/api.md) |
| Environment variables, webhooks, backup | [docs/configuration.md](docs/configuration.md) |
| Local development, security | [docs/development.md](docs/development.md) |

## License

MIT — see [LICENSE](LICENSE).
