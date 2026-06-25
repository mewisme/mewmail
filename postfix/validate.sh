#!/bin/sh
set -eu
DOMAIN=example.com
MAILHOST=mail.example.com
echo "$MAILHOST" > /etc/mailname
render() { sed -e "s|__DOMAIN__|${DOMAIN}|g" -e "s|__MAILHOST__|${MAILHOST}|g" "$1" >"$2"; }
render /etc/postfix/templates/main.cf /etc/postfix/main.cf
render /etc/postfix/templates/virtual_mailbox /etc/postfix/virtual_mailbox
postmap lmdb:/etc/postfix/virtual_mailbox
echo "postmap ok"
postfix set-permissions
echo "set-permissions ok"
postfix check
echo "postfix check ok"
result=$(postmap -q "user@${DOMAIN}" lmdb:/etc/postfix/virtual_mailbox)
echo "lookup: ${result}"
test -n "$result"
postconf -n virtual_mailbox_domains virtual_mailbox_maps virtual_transport local_transport default_transport
