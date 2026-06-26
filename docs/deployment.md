# Deployment

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

A background cleaner runs hourly and deletes emails older than `EMAIL_RETENTION_HOURS`. Attachment metadata is removed via foreign-key cascade. `VACUUM` runs approximately daily.

## Updating

```bash
git pull
docker compose pull
docker compose up -d
```

Data in `./data` persists across rebuilds.
