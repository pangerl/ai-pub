GOCACHE ?= $(CURDIR)/.cache/go-build

.PHONY: test web-check verify compose-up compose-down compose-check local-check

test:
	GOCACHE=$(GOCACHE) go test ./...

web-check:
	cd web && npm run lint && npm run build

verify: test web-check

compose-up:
	docker compose up --build -d

compose-down:
	docker compose --profile verify down -v --remove-orphans

compose-check:
	docker compose --profile verify down -v --remove-orphans
	docker compose --profile verify up --build --abort-on-container-exit --exit-code-from verify verify

local-check: compose-check
