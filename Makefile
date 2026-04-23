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

proto: ## Update go.mod to latest BSR-published proto versions (run after buf push in proto/)
	go get buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go@$(shell buf registry sdk version --module=buf.build/tldiagramcom/diagram --plugin=buf.build/protocolbuffers/go)
	go get buf.build/gen/go/tldiagramcom/diagram/connectrpc/go@$(shell buf registry sdk version --module=buf.build/tldiagramcom/diagram --plugin=buf.build/connectrpc/go)
	go mod tidy

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
