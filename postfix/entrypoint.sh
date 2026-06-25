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

echo "=== Postfix effective configuration (postconf -n) ==="
postconf -n
echo "=== virtual_mailbox map ==="
cat "$CONFIG_DIR/virtual_mailbox"
echo "====================================================="

exec postfix start-fg
