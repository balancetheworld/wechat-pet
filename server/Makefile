SHELL := /bin/sh

GO_FILES := $(shell find cmd internal -type d -name vendor -prune -o -type f -name '*.go' -print)

.PHONY: run build fmt fmt-check vet test check migrate-up migrate-down

run:
	go run ./cmd/api

build:
	mkdir -p bin
	go build -o bin/api ./cmd/api

fmt:
	gofmt -w $(GO_FILES)

fmt-check:
	@unformatted="$$(gofmt -l $(GO_FILES))"; \
	if [ -n "$$unformatted" ]; then \
		printf '%s\n' "$$unformatted"; \
		exit 1; \
	fi

vet:
	go vet ./...

test:
	go test ./...

check:
	$(MAKE) fmt-check
	$(MAKE) vet
	$(MAKE) test
	$(MAKE) build

migrate-up:
	./scripts/migrate-up.sh

migrate-down:
	./scripts/migrate-down.sh "$(MIGRATE_STEPS)"
