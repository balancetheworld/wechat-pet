SHELL := /bin/sh

GO := go
GOFMT := gofmt
APP := api
MIGRATE_STEPS ?= 1
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
	./scripts/migrate-up.sh

migrate-down:
	./scripts/migrate-down.sh "$(MIGRATE_STEPS)"
