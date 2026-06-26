# Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DOMAIN` | — | Mail domain for Postfix (catch-all) |
| `PORT` | `8080` | API listen port (host + container). Changing this requires `docker compose up -d --force-recreate` so Postfix picks up the new port (written to `/var/spool/postfix/.ingest-port` at startup). |
| `EMAIL_RETENTION_HOURS` | `168` | Auto-delete after N hours (default 7 days); kept emails are skipped |
| `WEBHOOK_URL` | — | Optional HTTP webhook for events (Discord-compatible) |
| `PUBLIC_URL` | — | Public base URL for webhook/UI links: `preview_url` (`/preview/{id}`) and `keep_url` (`/api/emails/{id}/keep`); also used by `POST /api/emails/{id}/preview-token` |

`API_HOST`, `DB_PATH` — optional overrides for the API (defaults: `0.0.0.0`, `/data/mail.db`). Not needed for Docker deploys.

## Webhooks

Set `WEBHOOK_URL` to receive notifications when emails are received, opened, or auto-cleaned.

- **Discord**: paste a channel webhook URL (`https://discord.com/api/webhooks/...`). Messages appear as embeds from **MewMail**.
- **Other endpoints**: receive JSON `{"event":"email.received"|"email.opened"|"email.cleaned","timestamp":"...","data":{...}}`.

When `PUBLIC_URL` is set, `email.received` payloads include one-click `preview_url` (UI at `/preview/{id}`) and `keep_url` (API at `/api/emails/{id}/keep`) with separate tokens (`preview_otk` is one-time; `keep_otk` survives preview). Preview links are consumed on first view; use `POST /api/emails/{id}/preview-token` to issue a new preview token.

Delivery is fire-and-forget; failures are logged and do not affect ingest or cleanup.

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
