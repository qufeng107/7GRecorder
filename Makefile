SHELL := /bin/sh

.PHONY: ci backend-ci frontend-ci system-ci backend-fmt backend-test backend-build frontend-install frontend-build

ci: backend-ci frontend-ci system-ci

backend-ci: backend-fmt backend-test backend-build

backend-fmt:
	cd backend && test -z "$$(gofmt -l .)"

backend-test:
	cd backend && go test ./...

backend-build:
	cd backend && go build -ldflags "-X github.com/7grecorder/7grecorder/backend/internal/version.BuildSHA=$${GITHUB_SHA:-dev}" -o ../dist/7grecorder ./cmd/7grecorder

frontend-ci: frontend-install
	cd frontend && pnpm lint
	cd frontend && pnpm typecheck
	cd frontend && pnpm test
	cd frontend && pnpm build

frontend-install:
	cd frontend && pnpm install --no-frozen-lockfile

system-ci:
	docker compose -f deploy/compose.yaml --env-file .env.example config >/dev/null
