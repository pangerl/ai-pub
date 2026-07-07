GOCACHE ?= $(CURDIR)/.cache/go-build
MYSQL_COMPOSE_FILES = -f deploy/compose.mysql.yaml -f deploy/compose.local-build.yaml
SQLITE_COMPOSE_FILES = -f deploy/compose.sqlite.yaml -f deploy/compose.local-build.yaml
DEMO_COMPOSE_FILES = --env-file .env -f deploy/compose.sqlite.yaml -f deploy/compose.demo.yaml

.PHONY: test web-check verify compose-up compose-down compose-check compose-sqlite-up compose-sqlite-down compose-check-sqlite demo-up demo-down local-check

test:
	GOCACHE=$(GOCACHE) go test ./...

web-check:
	cd web && npm run lint && npm run build

verify: test web-check

compose-up:
	docker compose $(MYSQL_COMPOSE_FILES) up --build -d

compose-down:
	docker compose $(MYSQL_COMPOSE_FILES) --profile verify down -v --remove-orphans

compose-check:
	docker compose $(MYSQL_COMPOSE_FILES) --profile verify down -v --remove-orphans
	APP_PORT=0 docker compose $(MYSQL_COMPOSE_FILES) --profile verify up --build --abort-on-container-exit --exit-code-from verify verify

compose-sqlite-up:
	docker compose $(SQLITE_COMPOSE_FILES) up --build -d

compose-sqlite-down:
	docker compose $(SQLITE_COMPOSE_FILES) --profile verify down -v --remove-orphans

compose-check-sqlite:
	docker compose $(SQLITE_COMPOSE_FILES) --profile verify down -v --remove-orphans
	APP_PORT=0 docker compose $(SQLITE_COMPOSE_FILES) --profile verify up --build --abort-on-container-exit --exit-code-from verify verify

demo-up:
	docker compose $(DEMO_COMPOSE_FILES) up -d

demo-down:
	docker compose $(DEMO_COMPOSE_FILES) down -v --remove-orphans

local-check: compose-check
