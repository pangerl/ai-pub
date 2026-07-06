GOCACHE ?= $(CURDIR)/.cache/go-build

.PHONY: test web-check verify compose-up compose-down compose-check compose-sqlite-up compose-sqlite-down compose-check-sqlite local-check

test:
	GOCACHE=$(GOCACHE) go test ./...

web-check:
	cd web && npm run lint && npm run build

verify: test web-check

compose-up:
	docker compose -f deploy/compose.mysql.yaml up --build -d

compose-down:
	docker compose -f deploy/compose.mysql.yaml --profile verify down -v --remove-orphans

compose-check:
	docker compose -f deploy/compose.mysql.yaml --profile verify down -v --remove-orphans
	APP_PORT=0 docker compose -f deploy/compose.mysql.yaml --profile verify up --build --abort-on-container-exit --exit-code-from verify verify

compose-sqlite-up:
	docker compose -f deploy/compose.sqlite.yaml up --build -d

compose-sqlite-down:
	docker compose -f deploy/compose.sqlite.yaml --profile verify down -v --remove-orphans

compose-check-sqlite:
	docker compose -f deploy/compose.sqlite.yaml --profile verify down -v --remove-orphans
	APP_PORT=0 docker compose -f deploy/compose.sqlite.yaml --profile verify up --build --abort-on-container-exit --exit-code-from verify verify

local-check: compose-check
