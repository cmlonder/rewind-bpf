BIN_DIR := bin
BIN := $(BIN_DIR)/rewind
EVIDENCE_BIN := $(BIN_DIR)/rewind-evidence

.PHONY: build test fmt clean release bootstrap

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN) ./cmd/rewind
	go build -o $(EVIDENCE_BIN) ./cmd/rewind-evidence

test:
	go test ./...

fmt:
	gofmt -w $$(find cmd internal -name '*.go' -type f 2>/dev/null)

clean:
	rm -rf $(BIN_DIR)

release:
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o $(BIN_DIR)/rewind-linux-amd64 ./cmd/rewind
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o $(BIN_DIR)/rewind-linux-arm64 ./cmd/rewind
	GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o $(BIN_DIR)/rewind-darwin-arm64 ./cmd/rewind
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o $(BIN_DIR)/rewind-windows-amd64.exe ./cmd/rewind

bootstrap:
	./scripts/bootstrap_vm.sh
