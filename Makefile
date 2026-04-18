BINARY             := beehiiv-mcp
# Default to the first valid code-signing identity in your keychain. Override
# with `make install CODESIGN_IDENTITY="SHA1 or name"` if you have multiple.
CODESIGN_IDENTITY  ?= $(shell security find-identity -v -p codesigning 2>/dev/null | awk 'NR==1 {print $$2}')

.PHONY: build
build:
	go build -o $(BINARY) .

.PHONY: sign
sign: build
	@if [ -z "$(CODESIGN_IDENTITY)" ]; then \
		echo ""; \
		echo "error: no code-signing identity found in your keychain."; \
		echo ""; \
		echo "Options:"; \
		echo "  (a) Use an Apple Developer cert (recommended if you have one):"; \
		echo "      security find-identity -v -p codesigning"; \
		echo "      make install CODESIGN_IDENTITY=\"<SHA1 or full name>\""; \
		echo ""; \
		echo "  (b) Create a self-signed cert in Keychain Access → Certificate"; \
		echo "      Assistant → Create a Certificate... (Self-Signed Root, Code Signing)."; \
		echo "      Then trust it for codesigning via:"; \
		echo "      security add-trusted-cert -d -r trustRoot -p codeSign -k \\"; \
		echo "        ~/Library/Keychains/login.keychain-db <path-to-cert.pem>"; \
		exit 1; \
	fi
	@echo "Signing $(BINARY) with identity $(CODESIGN_IDENTITY)"
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
