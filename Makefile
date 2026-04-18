BINARY             := beehiiv-mcp
CODESIGN_IDENTITY  ?= beehiiv-mcp-dev

.PHONY: build
build:
	go build -o $(BINARY) .

.PHONY: sign
sign: build
	@if ! security find-identity -v -p codesigning | grep -q "$(CODESIGN_IDENTITY)"; then \
		echo ""; \
		echo "error: code-signing identity '$(CODESIGN_IDENTITY)' not found."; \
		echo ""; \
		echo "Create one in Keychain Access:"; \
		echo "  Keychain Access → Certificate Assistant → Create a Certificate..."; \
		echo "  - Name: $(CODESIGN_IDENTITY)"; \
		echo "  - Identity Type: Self-Signed Root"; \
		echo "  - Certificate Type: Code Signing"; \
		echo ""; \
		echo "Or override: make install CODESIGN_IDENTITY=\"Your Identity Name\""; \
		exit 1; \
	fi
	codesign --sign "$(CODESIGN_IDENTITY)" --force --options runtime $(BINARY)

.PHONY: install
install: sign
	@echo ""
	@echo "Built and signed $(BINARY) at $(abspath $(BINARY))"
	@echo "Add to Claude Code MCP config:"
	@echo ""
	@echo '  {'
	@echo '    "mcpServers": {'
	@echo '      "beehiiv": { "command": "$(abspath $(BINARY))" }'
	@echo '    }'
	@echo '  }'

.PHONY: test
test:
	go test ./...

.PHONY: test-integration
test-integration:
	go test -tags integration ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: cover
cover:
	go test -cover ./...

.PHONY: clean
clean:
	rm -f $(BINARY) coverage.out
