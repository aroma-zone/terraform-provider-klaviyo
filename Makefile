## Klaviyo Terraform provider — build, fetch, test, docs.
##
## All commands run from this directory (providers/klaviyo).
## Developer tools install locally to ./bin so this provider's
## toolchain is independent from the rest of the monorepo.

SHELL          := /usr/bin/env bash
PROVIDER_NAME  := klaviyo
GOBIN          := $(CURDIR)/bin
PATH           := $(GOBIN):$(PATH)
export GOBIN PATH

# Pinned developer tools (bump intentionally).
TFPLUGINDOCS := v0.25.0

# Pinned upstream spec. To bump: edit spec/version.txt, then `make fetch-spec`.
SPEC_COMMIT := $(shell awk -F': *' '/^commit:/ {print $$2}' spec/version.txt)
SPEC_URL    := https://raw.githubusercontent.com/klaviyo/openapi/$(SPEC_COMMIT)/openapi/stable.json

.PHONY: help tools fetch-spec build test docs install clean

help:  ## Show available targets
	@awk 'BEGIN{FS=":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

tools:  ## Install pinned developer tools (tfplugindocs) into ./bin
	@mkdir -p $(GOBIN)
	go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@$(TFPLUGINDOCS)

fetch-spec:  ## Re-download the upstream Klaviyo OpenAPI spec at the pinned commit
	@echo "Fetching Klaviyo spec @ $(SPEC_COMMIT)"
	curl -sSLf -o spec/openapi.json $(SPEC_URL)
	@echo "Wrote spec/openapi.json ($$(wc -c < spec/openapi.json) bytes)"

build:  ## Compile the provider binary into ./bin/
	go build -o bin/terraform-provider-$(PROVIDER_NAME) .

test:  ## Run unit tests
	go test ./...

docs: tools  ## Generate Terraform Registry docs from the compiled provider
	$(GOBIN)/tfplugindocs generate --provider-name $(PROVIDER_NAME)

clean:  ## Remove build artifacts and locally installed tools
	rm -rf bin/ dist/
