TASK = go tool task

.PHONY: setup-hooks frontend-deps frontend-build lint-be lint-fe be fe dev dev-stop proto test test-be test-fe test-e2e build go run clean wails-build wails-dev

setup-hooks:
	$(TASK) setup-hooks

frontend-deps:
	$(TASK) frontend-deps

frontend-build:
	$(TASK) frontend-build

lint-be:
	$(TASK) lint-be

lint-fe:
	$(TASK) lint-fe

be:
	$(TASK) be

fe:
	$(TASK) fe

dev:
	$(TASK) dev

dev-stop:
	$(TASK) dev-stop

proto:
	$(TASK) proto

test:
	$(TASK) test

test-be:
	$(TASK) test-be

test-fe:
	$(TASK) test-fe

test-e2e:
	$(TASK) test-e2e

build:
	$(TASK) build

go:
	$(TASK) go

run:
	$(TASK) run

wails-build:
	$(TASK) wails-build

wails-dev:
	$(TASK) wails-dev

clean:
	$(TASK) clean
