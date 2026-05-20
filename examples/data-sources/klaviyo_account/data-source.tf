terraform {
  required_providers {
    klaviyo = { source = "aroma-zone/klaviyo", version = "~> 0.1" }
  }
}
provider "klaviyo" {}

# No `id` set: returns the single account the API key is scoped to.
data "klaviyo_account" "current" {}

output "account_id" {
  value = data.klaviyo_account.current.id
}

output "account_currency" {
  value = data.klaviyo_account.current.preferred_currency
}

output "account_timezone" {
  value = data.klaviyo_account.current.timezone
}

output "account_public_key" {
  value       = data.klaviyo_account.current.public_api_key
  description = "Safe to expose to client-side code."
}
