MODULE  := github.com/nlink-jp/voice-studio-mcp
BINARY  := voice-studio-mcp
BIN_DIR := dist

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X $(MODULE)/cmd.Version=$(VERSION)"

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

.PHONY: build build-all package test test-e2e install-skill uninstall-skill clean help

## build: Build binary for the current OS/Arch → ./dist/voice-studio-mcp
build:
	@mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY) .
	@scripts/codesign-darwin.sh $(BIN_DIR)/$(BINARY) "$(CODESIGN_IDENTITY)"

## build-all: Cross-compile and codesign each darwin build
build-all:
	@mkdir -p $(BIN_DIR)
	$(foreach platform,$(PLATFORMS),$(call build_platform,$(platform)))

define build_platform
	$(eval OS   := $(word 1,$(subst /, ,$(1))))
	$(eval ARCH := $(word 2,$(subst /, ,$(1))))
	$(eval EXT  := $(if $(filter windows,$(OS)),.exe,))
	$(eval OUT  := $(BIN_DIR)/$(BINARY)-$(OS)-$(ARCH)$(EXT))
	@echo "Building $(OUT)..."
	GOOS=$(OS) GOARCH=$(ARCH) go build $(LDFLAGS) -o $(OUT) .
	@scripts/codesign-darwin.sh $(OUT) "$(CODESIGN_IDENTITY)"

endef

## package: Cross-compile, codesign, zip, and notarize darwin builds
package: build-all
	$(foreach platform,$(PLATFORMS), \
		$(eval OS   := $(word 1,$(subst /, ,$(platform)))) \
		$(eval ARCH := $(word 2,$(subst /, ,$(platform)))) \
		$(eval EXT  := $(if $(filter windows,$(OS)),.exe,)) \
		$(eval BIN  := $(BIN_DIR)/$(BINARY)-$(OS)-$(ARCH)$(EXT)) \
		$(eval ZIP  := $(BIN_DIR)/$(BINARY)-$(VERSION)-$(OS)-$(ARCH).zip) \
		$(eval STAGE := $(BIN_DIR)/_pkg-$(OS)-$(ARCH)) \
		rm -rf $(STAGE) && mkdir -p $(STAGE) ; \
		cp $(BIN) $(STAGE)/$(BINARY)$(EXT) ; \
		cp README.md $(STAGE)/README.md ; \
		zip -j $(ZIP) $(STAGE)/$(BINARY)$(EXT) $(STAGE)/README.md ; \
		rm -rf $(STAGE) ;)
	@scripts/notarize-darwin.sh $(BIN_DIR)/$(BINARY)-$(VERSION)-darwin-arm64.zip "$(NOTARY_PROFILE)"

## test: Run all unit tests
test:
	go test ./...

## test-e2e: Run e2e tests against a freshly built binary (mock engine; no AivisSpeech needed)
test-e2e: build
	VOICE_STUDIO_TEST_BINARY=$(abspath $(BIN_DIR)/$(BINARY)) go test -tags e2e ./e2e/...

SKILLS_DEST ?= $(HOME)/.claude/skills

## install-skill: Install the bundled radio-drama skill into ~/.claude/skills
install-skill:
	@mkdir -p $(SKILLS_DEST)
	cp -R skills/radio-drama $(SKILLS_DEST)/
	@echo "installed: radio-drama -> $(SKILLS_DEST)/radio-drama"

## uninstall-skill: Remove the installed radio-drama skill
uninstall-skill:
	rm -rf $(SKILLS_DEST)/radio-drama
	@echo "removed: $(SKILLS_DEST)/radio-drama"

## clean: Remove build artifacts
clean:
	rm -rf $(BIN_DIR)

## help: Show available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'
