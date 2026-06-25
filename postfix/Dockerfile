FROM alpine:3.21

RUN apk add --no-cache postfix curl ca-certificates \
    && mkdir -p /var/spool/postfix/pid /var/spool/postfix/private /var/spool/postfix/public

COPY main.cf /etc/postfix/main.cf
COPY master.cf /etc/postfix/master.cf
COPY virtual /etc/postfix/virtual
COPY transport /etc/postfix/transport
COPY ingest-mail /usr/local/bin/ingest-mail
COPY entrypoint.sh /entrypoint.sh

RUN chmod +x /usr/local/bin/ingest-mail /entrypoint.sh

EXPOSE 25
ENTRYPOINT ["/entrypoint.sh"]
