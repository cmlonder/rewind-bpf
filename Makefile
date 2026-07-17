BIN_DIR := bin
BIN := $(BIN_DIR)/rewind

.PHONY: build test fmt clean

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN) ./cmd/rewind

test:
	go test ./...

fmt:
	gofmt -w $$(find cmd internal -name '*.go' -type f 2>/dev/null)

clean:
	rm -rf $(BIN_DIR)
