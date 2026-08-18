SHELL := /bin/sh

GO := go
GOFMT := gofmt
APP := api
MIGRATE := migrate
MIGRATIONS_DIR ?= db/migrations
MIGRATE_STEPS ?= 1
DATABASE_URL ?=
GO_FILES := $(shell find . -type d -name vendor -prune -o -type f -name '*.go' -print)

.PHONY: run build binary fmt fmt-check vet test check migrate-up migrate-down

run:
	$(GO) run ./cmd/$(APP)

build:
	mkdir -p bin
	$(GO) build -o bin/$(APP) ./cmd/$(APP)

# for internal use
# typically called from Dockerfiles
binary:
	cd cmd/$(WHAT) && $(GO) build -o ../../bin/$(WHAT) .

fmt:
	$(GOFMT) -w $(GO_FILES)

fmt-check:
	@unformatted="$$($(GOFMT) -l $(GO_FILES))"; \
	if [ -n "$$unformatted" ]; then \
		printf '%s\n' "$$unformatted"; \
		exit 1; \
	fi

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

check: fmt-check vet test build

migrate-up:
	@test -n "$(DATABASE_URL)" || { echo "DATABASE_URL is required"; exit 1; }
	$(MIGRATE) -path "$(MIGRATIONS_DIR)" -database "$(DATABASE_URL)" up

migrate-down:
	@test -n "$(DATABASE_URL)" || { echo "DATABASE_URL is required"; exit 1; }
	$(MIGRATE) -path "$(MIGRATIONS_DIR)" -database "$(DATABASE_URL)" down $(MIGRATE_STEPS)
