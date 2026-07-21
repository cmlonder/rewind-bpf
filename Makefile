BIN_DIR := bin
BIN := $(BIN_DIR)/rewind
EVIDENCE_BIN := $(BIN_DIR)/rewind-evidence

.PHONY: build test fmt clean release release-manifest release-sign release-bundle release-preflight hackathon-preflight public-audit publish-site benchmark-verify evidence-bundle ui-smoke site-smoke dashboard-smoke bootstrap acceptance-vm supervisor-smoke-vm policy-bundle-smoke-vm p1-leak-smoke-vm mac-safe-smoke mac-native-smoke mac-crash-smoke platform-status jury-demo-vm final-vm

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

release-manifest: release
	./scripts/release_manifest.sh $(BIN_DIR)

release-sign: release-manifest
	test -n "$(REWIND_RELEASE_PRIVATE_KEY)" || (echo "REWIND_RELEASE_PRIVATE_KEY is required" >&2; exit 2)
	go run ./cmd/rewind release sign --input $(BIN_DIR)/SHA256SUMS --private-key "$(REWIND_RELEASE_PRIVATE_KEY)" --output $(BIN_DIR)/SHA256SUMS.sig
	./scripts/mark_release_signed.sh $(BIN_DIR)/release-metadata.txt SHA256SUMS.sig

release-bundle: release-manifest
	./scripts/release_bundle.sh $(BIN_DIR)

release-preflight:
	./scripts/release_preflight.sh

hackathon-preflight:
	./scripts/hackathon_preflight.sh

public-audit:
	./scripts/public_repo_audit.sh

publish-site:
	./scripts/publish_site.sh site

benchmark-verify:
	python3 benchmarks/normalize_results.py
	python3 benchmarks/plot_results.py
	./scripts/benchmark_verify.sh benchmarks

evidence-bundle:
	./scripts/final_evidence_bundle.sh

ui-smoke:
	./scripts/ui_smoke.sh

dashboard-smoke:
	./scripts/dashboard_smoke.sh

site-smoke:
	./scripts/site_smoke.sh

bootstrap:
	./scripts/bootstrap_vm.sh

acceptance-vm:
	./scripts/acceptance_vm.sh

supervisor-smoke-vm:
	./scripts/supervisor_smoke_vm.sh

policy-bundle-smoke-vm:
	./scripts/policy_bundle_smoke_vm.sh

p1-leak-smoke-vm:
	REWIND_VM_CONFIRM=VM_ONLY ./scripts/p1_leak_smoke_vm.sh

mac-safe-smoke:
	./scripts/mac_safe_smoke.sh

mac-native-smoke:
	./scripts/mac_native_smoke.sh

mac-crash-smoke:
	./scripts/macos_crash_smoke.sh

platform-status:
	go run ./cmd/rewind platform status

jury-demo-vm:
	./scripts/jury_demo_vm.sh

final-vm:
	REWIND_VM_CONFIRM=VM_ONLY ./scripts/final_vm_gate.sh
