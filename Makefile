.PHONY: dev build test clean

dev:
	@echo "Starting Go API on :8420 and Vite on :5173"
	@(cd web && bun run dev) & go run ./cmd/souschef

build:
	cd web && bun install --frozen-lockfile && bun run build
	go build -ldflags "-X main.version=$$(git describe --tags --always --dirty)" -o bin/souschef ./cmd/souschef
	@echo "Built bin/souschef"

test:
	go test ./... -race
	cd web && bunx playwright test

clean:
	rm -rf bin web/dist
