#!/bin/sh
set -eu

DOMAIN="${DOMAIN:?DOMAIN required}"

sed -i "s/example.com/${DOMAIN}/g" /etc/postfix/main.cf
sed -i "s/example.com/${DOMAIN}/g" /etc/postfix/virtual

postmap /etc/postfix/virtual
postmap /etc/postfix/transport
postconf -e "myhostname=mail.${DOMAIN}"

exec postfix start-fg
