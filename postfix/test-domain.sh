#!/bin/sh
set -eu
DOMAIN="${DOMAIN:?DOMAIN required}"
DOMAIN=$(printf '%s' "$DOMAIN" | tr -d '\r' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
echo "mail.${DOMAIN}" > /etc/mailname
postconf -e "myhostname=mail.${DOMAIN}"
postconf -e "virtual_alias_domains=${DOMAIN}"
printf '@%s catchall@localhost\n' "$DOMAIN" > /etc/postfix/virtual
postmap /etc/postfix/virtual
postconf myhostname virtual_alias_domains
cat /etc/postfix/virtual
