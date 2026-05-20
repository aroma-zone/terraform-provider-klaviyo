## Klaviyo Terraform provider — build, generate, test
##
## All commands run from this directory (providers/klaviyo).
## Tools are installed locally to ./bin so this provider's
## toolchain is independent from the rest of the monorepo.

SHELL                 := /usr/bin/env bash
PROVIDER_NAME         := klaviyo
GOBIN                 := $(CURDIR)/bin
PATH                  := $(GOBIN):$(PATH)
export GOBIN PATH

# Pinned tool versions (bump intentionally).
TFPLUGINGEN_OPENAPI    := v0.3.0
TFPLUGINGEN_FRAMEWORK  := v0.4.1
TFPLUGINDOCS           := v0.25.0

# Pinned spec source. To bump, edit spec/version.txt then `make fetch-spec`.
SPEC_REVISION := $(shell awk -F': *' '/^revision:/ {print $$2}' spec/version.txt)
SPEC_COMMIT   := $(shell awk -F': *' '/^commit:/   {print $$2}' spec/version.txt)
SPEC_URL      := https://raw.githubusercontent.com/klaviyo/openapi/$(SPEC_COMMIT)/openapi/stable.json

.PHONY: help tools fetch-spec generate generate-codespec generate-framework build test clean

help:  ## Show available targets
	@awk 'BEGIN{FS=":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

tools:  ## Install pinned codegen tools to ./bin
	@mkdir -p $(GOBIN)
	go install github.com/hashicorp/terraform-plugin-codegen-openapi/cmd/tfplugingen-openapi@$(TFPLUGINGEN_OPENAPI)
	go install github.com/hashicorp/terraform-plugin-codegen-framework/cmd/tfplugingen-framework@$(TFPLUGINGEN_FRAMEWORK)
	go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@$(TFPLUGINDOCS)

fetch-spec:  ## Re-download the upstream Klaviyo OpenAPI spec at the pinned commit
	@echo "Fetching Klaviyo spec @ $(SPEC_REVISION) ($(SPEC_COMMIT))"
	curl -sSLf -o spec/openapi.json $(SPEC_URL)
	@echo "Wrote spec/openapi.json ($$(wc -c < spec/openapi.json) bytes)"

generate-codespec:  ## OpenAPI -> Provider Code Specification
	tfplugingen-openapi generate \
		--config spec/generator-config.yaml \
		--output spec/provider-code-spec.json \
		spec/openapi.json

generate-framework:  ## Provider Code Specification -> Plugin Framework scaffolding
	tfplugingen-framework generate all \
		--input spec/provider-code-spec.json \
		--output internal

generate: generate-codespec generate-framework  ## Full codegen pipeline

build:  ## Compile the provider binary
	go build -o bin/terraform-provider-$(PROVIDER_NAME) .

test:  ## Run unit tests
	go test ./...

clean:  ## Remove build artifacts and local tools
	rm -rf bin/ dist/
