terraform {
  required_providers {
    klaviyo = { source = "aroma-zone/klaviyo", version = "~> 0.1" }
  }
}
provider "klaviyo" {}

variable "webhook_secret" {
  type      = string
  sensitive = true
}

resource "klaviyo_webhook" "sms_events" {
  name         = "SMS Events"
  description  = "Forwards SMS delivery events to our backend."
  endpoint_url = "https://my-app.example.com/klaviyo/webhook"
  secret_key   = var.webhook_secret
  enabled      = true

  webhook_topics = [
    "event:klaviyo.sent_sms",
    "event:klaviyo.failed_sms",
  ]
}
