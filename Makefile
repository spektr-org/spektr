# Spektr — Build targets
# Run: make help

.PHONY: help build wasm npm test clean

BINARY    = bin/spektr
WASM_OUT  = bin/spektr.wasm
NPM_DIR   = npm
GO_MODULE = github.com/spektr-org/spektr

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build CLI binary
	go build -o $(BINARY) ./cmd/spektr/
	@echo "✅ Built $(BINARY)"

wasm: ## Build WASM binary
	GOOS=js GOARCH=wasm go build -o $(WASM_OUT) ./cmd/wasm/
	@echo "✅ Built $(WASM_OUT) ($$(du -h $(WASM_OUT) | cut -f1))"

npm: wasm ## Build npm package (copy WASM + wasm_exec.js)
	cp $(WASM_OUT) $(NPM_DIR)/spektr.wasm
	cp "$$(go env GOROOT)/misc/wasm/wasm_exec.js" $(NPM_DIR)/wasm_exec.js
	@echo "✅ npm package ready in $(NPM_DIR)/"
	@echo "   To publish: cd $(NPM_DIR) && npm publish --access public"

test: ## Run all tests
	go test ./... -v -count=1
	@echo ""
	@echo "✅ All Go tests passed"

test-npm: npm ## Run npm package test (requires Node.js)
	cd $(NPM_DIR) && node test.js
	@echo ""
	@echo "✅ npm test passed"

clean: ## Remove build artifacts
	rm -f $(BINARY) $(WASM_OUT)
	rm -f $(NPM_DIR)/spektr.wasm $(NPM_DIR)/wasm_exec.js
	@echo "✅ Cleaned"

all: build wasm npm test test-npm ## Build everything and run all tests