.PHONY: frontend-deps frontend-build build run clean dev test-backend build-go

frontend-deps:
	if [ ! -d frontend/node_modules ]; then cd frontend && npm install; fi

frontend-build: frontend-deps
	cd frontend && VITE_APP_BASE=/ VITE_ROUTER_BASENAME=/ npm run build

dev: frontend-deps
	@echo "Starting development stack..."
	@(cd frontend && npm run dev) & \
	DEV=true air; \
	wait
test: test-backend
	go test ./...

build: frontend-build
	go build -o $(shell go env GOPATH)/bin/tld ./cmd/tld

go: build-go
	go build -o $(shell go env GOPATH)/bin/tld ./cmd/tld

run: frontend-build
	go run ./cmd/tld serve

clean:
	rm -f tld
