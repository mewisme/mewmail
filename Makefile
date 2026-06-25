.PHONY: test build up up-build down logs backup-db api-curl api-health pull

COMPOSE = docker compose
API_DIR = api

test:
	cd $(API_DIR) && go test ./...

build:
	cd $(API_DIR) && go build -o ../bin/server ./cmd/server

pull:
	$(COMPOSE) pull

up:
	$(COMPOSE) pull
	$(COMPOSE) up -d

up-build:
	$(COMPOSE) up -d --build

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f

backup-db:
	@mkdir -p backups
	cp data/mail.db backups/mail-$$(date +%Y%m%d%H%M%S).db 2>/dev/null || \
		(echo "mail.db not found — start the stack first" && exit 1)
	@echo "Backup written to backups/"

api-curl:
	$(COMPOSE) exec postfix sh -c 'curl -fsS http://api:$${PORT:-8080}/health'

api-health: api-curl
