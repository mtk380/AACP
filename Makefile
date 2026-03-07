.PHONY: build test proto lint e2e fmt demo-ui

GO ?= go

build:
	$(GO) build ./...

test:
	$(GO) test ./...

proto:
	@if command -v buf >/dev/null 2>&1; then \
		buf generate; \
	elif command -v protoc >/dev/null 2>&1; then \
		protoc -I proto --go_out=. --go-grpc_out=. proto/aacp/v1/*.proto; \
	else \
		echo "buf/protoc not found"; exit 1; \
	fi

lint:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

e2e:
	$(GO) test ./tests/e2e/...

demo-ui:
	./scripts/demo-ui.sh
