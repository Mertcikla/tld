TASK = go tool task

.PHONY: setup-hooks frontend-deps frontend-build lint-be lint-fe be fe dev dev-stop proto test test-be test-fe test-e2e build go run clean wails-build wails-dev embed-server

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


EMBED_PORT ?= 8081
EMBED_MODEL ?= $(HOME)/.cache/huggingface/hub/models--jinaai--jina-code-embeddings-0.5b-GGUF/snapshots/*/jina-code-embeddings-0.5b-F16.gguf
EMBED_LOG ?= $(HOME)/.cache/llama-embed.log

embed-server:
	@if curl -sf http://127.0.0.1:$(EMBED_PORT)/health >/dev/null 2>&1; then \
		echo "embedding server already live on http://127.0.0.1:$(EMBED_PORT)"; \
		exit 0; \
	fi; \
	if ! command -v llama-server >/dev/null 2>&1; then \
		echo "llama-server not found (install with: brew install llama.cpp)"; exit 1; \
	fi; \
	if [ -z "$$(ls $(EMBED_MODEL) 2>/dev/null)" ]; then \
		echo "model not found: $(EMBED_MODEL)"; \
		echo "download it with: huggingface-cli download jinaai/jina-code-embeddings-0.5b-GGUF jina-code-embeddings-0.5b-F16.gguf"; \
		exit 1; \
	fi; \
	echo "starting llama-server on http://127.0.0.1:$(EMBED_PORT)"; \
	nohup llama-server -m $(EMBED_MODEL) --host 127.0.0.1 --port $(EMBED_PORT) \
		--embeddings --pooling last --alias jina-code-embeddings-0.5b \
		-c 8192 -b 2048 -ub 2048 > $(EMBED_LOG) 2>&1 & \
	echo "llama-server pid $$! (log $(EMBED_LOG))"
