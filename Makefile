MODULE  := github.com/nlink-jp/voice-studio-mcp
BINARY  := voice-studio-mcp
BIN_DIR := dist

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X $(MODULE)/cmd.Version=$(VERSION)"

# macOS Developer ID signing / notarization (see CONVENTIONS.md §Code
# Signing). Defaults match any Developer ID Application cert in the
# keychain and the org-standard notary profile. Builds without these
# fall back to ad-hoc / un-notarized with a one-line warning.
CODESIGN_IDENTITY ?= Developer ID Application
NOTARY_PROFILE    ?= nlink-jp-notary

# v1 targets macOS on Apple Silicon only (engine lifecycle management is
# darwin-specific); the cross-compile scaffolding is kept for the future.
PLATFORMS := \
	darwin/arm64

.PHONY: build build-all package test test-e2e package-skill install-skill uninstall-skill clean help

## build: Build binary for the current OS/Arch → ./dist/voice-studio-mcp
build:
	@mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY) .
	@scripts/codesign-darwin.sh $(BIN_DIR)/$(BINARY) "$(CODESIGN_IDENTITY)"

## build-all: Cross-compile and codesign each darwin build
build-all:
	@mkdir -p $(BIN_DIR)
	@for p in $(PLATFORMS); do os=$${p%/*}; arch=$${p#*/}; \
		ext=""; [ "$$os" = windows ] && ext=".exe"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY)-$$os-$$arch$$ext . ; \
	done
	@scripts/codesign-darwin.sh $(BIN_DIR)/$(BINARY)-darwin-arm64 "$(CODESIGN_IDENTITY)" "$(BINARY)"

## package: Build all platforms, archive with version suffix (zip for
## darwin/windows, tar.gz for linux), bundle the canonical binary +
## README.md + LICENSE, and notarize the darwin build. Asset naming
## follows the org Release Archive Standard
## (voice-studio-mcp-vX.Y.Z-<os>-<arch>.<ext>). v1 ships darwin/arm64 only.
package: build-all
	@cd $(BIN_DIR) && for p in $(PLATFORMS); do os=$${p%/*}; arch=$${p#*/}; \
		ext=""; [ "$$os" = windows ] && ext=".exe"; \
		stage=_pkg; rm -rf $$stage; mkdir -p $$stage; \
		cp "$(BINARY)-$$os-$$arch$$ext" "$$stage/$(BINARY)$$ext"; \
		cp ../README.md ../LICENSE $$stage/; \
		base="$(BINARY)-$(VERSION)-$$os-$$arch"; \
		if [ "$$os" = linux ]; then ( cd $$stage && tar -czf "../$$base.tar.gz" * ); \
		else ( cd $$stage && zip -q "../$$base.zip" * ); fi; \
		rm -rf $$stage; \
	done
	@scripts/notarize-darwin.sh $(BIN_DIR)/$(BINARY)-$(VERSION)-darwin-arm64.zip "$(NOTARY_PROFILE)"

## test: Run all unit tests
test:
	go test ./...

## test-e2e: Run e2e tests against a freshly built binary (mock engine; no AivisSpeech needed)
test-e2e: build
	VOICE_STUDIO_TEST_BINARY=$(abspath $(BIN_DIR)/$(BINARY)) go test -tags e2e ./e2e/...

SKILLS_DEST ?= $(HOME)/.claude/skills
SKILL_NAME  := multi-actor-narration

## package-skill: Build the bundled skill into dist/multi-actor-narration.skill
package-skill:
	@bash skills/build.sh

## install-skill: Install the bundled multi-actor-narration skill into ~/.claude/skills
install-skill:
	@mkdir -p $(SKILLS_DEST)
	rm -rf $(SKILLS_DEST)/$(SKILL_NAME)
	cp -R skills/$(SKILL_NAME) $(SKILLS_DEST)/
	@echo "installed: $(SKILL_NAME) -> $(SKILLS_DEST)/$(SKILL_NAME)"

## uninstall-skill: Remove the installed multi-actor-narration skill
uninstall-skill:
	rm -rf $(SKILLS_DEST)/$(SKILL_NAME)
	@echo "removed: $(SKILLS_DEST)/$(SKILL_NAME)"

## clean: Remove build artifacts
clean:
	rm -rf $(BIN_DIR)

## help: Show available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'
