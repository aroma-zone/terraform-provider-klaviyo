terraform {
  required_providers {
    klaviyo = {
      source  = "aroma-zone/klaviyo"
      version = "~> 0.1"
    }
  }
}

# Recommended: feed credentials through environment variables so they
# don't end up in tfstate or shell history.
#   export KLAVIYO_API_KEY="pk_..."
#   export KLAVIYO_API_REVISION="2026-04-15.pre"  # optional override
provider "klaviyo" {
  # api_key, api_revision, and base_url can all be set here too if
  # you prefer explicit configuration.
}
