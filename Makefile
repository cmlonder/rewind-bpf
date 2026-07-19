BIN_DIR := bin
BIN := $(BIN_DIR)/rewind
EVIDENCE_BIN := $(BIN_DIR)/rewind-evidence

.PHONY: build test fmt clean

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
