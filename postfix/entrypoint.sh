#!/bin/sh
set -eu

DOMAIN="${DOMAIN:?DOMAIN required}"

echo "mail.${DOMAIN}" > /etc/mailname

sed -i "s/example.com/${DOMAIN}/g" /etc/postfix/main.cf
sed -i "s/example.com/${DOMAIN}/g" /etc/postfix/virtual

postmap /etc/postfix/virtual
postmap /etc/postfix/transport
postconf -e "myhostname=mail.${DOMAIN}"

# set-permissions needs config + mailname; skip failure on overlay/build quirks
postfix set-permissions 2>/dev/null || true
postfix check

exec postfix start-fg
