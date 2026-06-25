#!/bin/sh
set -eu

DOMAIN="${DOMAIN:?DOMAIN required}"
DOMAIN=$(printf '%s' "$DOMAIN" | tr -d '\r' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
[ -n "$DOMAIN" ] || { echo "DOMAIN is empty" >&2; exit 1; }

MAILHOST="mail.${DOMAIN}"
TEMPLATE_DIR=/etc/postfix/templates
CONFIG_DIR=/etc/postfix

echo "$MAILHOST" > /etc/mailname

render() {
	# ponytail: domains are hostnames; | delimiter avoids clashes with /
	sed -e "s|__DOMAIN__|${DOMAIN}|g" -e "s|__MAILHOST__|${MAILHOST}|g" "$1" >"$2"
}

render "$TEMPLATE_DIR/main.cf" "$CONFIG_DIR/main.cf"
render "$TEMPLATE_DIR/virtual_mailbox" "$CONFIG_DIR/virtual_mailbox"

postmap lmdb:"$CONFIG_DIR/virtual_mailbox"

# Fail fast if catch-all map entry is missing (Alpine uses LMDB, not hash)
if ! postmap -q "@${DOMAIN}" lmdb:"$CONFIG_DIR/virtual_mailbox" | grep -q .; then
	echo "virtual_mailbox catch-all missing for @${DOMAIN}" >&2
	exit 1
fi

if [ -f /etc/aliases ] && [ -s /etc/aliases ] && grep -qvE '^\s*($|#)' /etc/aliases; then
	newaliases
fi

postfix set-permissions
postfix check

# Copy api_key for pipe user (nobody cannot read 0600 files from api container)
ingest_refresh_token() {
	CRED="${CREDENTIALS_PATH:-/data/.credentials}"
	TOKEN_FILE=/var/spool/postfix/.ingest-token
	if [ ! -f "$CRED" ]; then
		echo "WARN: $CRED not found — start api first, then restart postfix" >&2
		return 0
	fi
	TOKEN=$(sed -n 's/.*"api_key"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$CRED" | head -1)
	if [ -z "$TOKEN" ]; then
		echo "WARN: api_key missing in $CRED" >&2
		return 0
	fi
	printf '%s' "$TOKEN" >"$TOKEN_FILE"
	chown nobody:nobody "$TOKEN_FILE"
	chmod 400 "$TOKEN_FILE"
	echo "ingest token ready at $TOKEN_FILE" >&2
}
ingest_refresh_token

API_PORT="${PORT:-8080}"
API_HOST="${API_SERVICE_HOST:-api}"
PORT_FILE=/var/spool/postfix/.ingest-port
HOST_FILE=/var/spool/postfix/.ingest-host
printf '%s' "$API_PORT" >"$PORT_FILE"
printf '%s' "$API_HOST" >"$HOST_FILE"
chown nobody:nobody "$PORT_FILE" "$HOST_FILE"
chmod 444 "$PORT_FILE" "$HOST_FILE"
echo "ingest target http://${API_HOST}:${API_PORT}/internal/ingest" >&2

if ! curl -fsS --max-time 5 "http://${API_HOST}:${API_PORT}/health" >/dev/null; then
	echo "WARN: api not reachable at http://${API_HOST}:${API_PORT}/health — mail will queue until api is up" >&2
fi

echo "=== Postfix effective configuration (postconf -n) ==="
postconf -n
echo "=== virtual_mailbox map ==="
cat "$CONFIG_DIR/virtual_mailbox"
echo "====================================================="

exec postfix start-fg
