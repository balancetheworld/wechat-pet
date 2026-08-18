SHELL := /bin/sh

.PHONY: run build fmt fmt-check vet test check migrate-up migrate-down

run:
	$(MAKE) -C server run

build:
	$(MAKE) -C server build

fmt:
	$(MAKE) -C server fmt

fmt-check:
	$(MAKE) -C server fmt-check

vet:
	$(MAKE) -C server vet

test:
	$(MAKE) -C server test

check:
	$(MAKE) -C server check

migrate-up:
	$(MAKE) -C server migrate-up

migrate-down:
	$(MAKE) -C server migrate-down
