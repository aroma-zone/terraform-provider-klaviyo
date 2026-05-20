terraform {
  required_providers {
    klaviyo = { source = "aroma-zone/klaviyo", version = "~> 0.1" }
  }
}
provider "klaviyo" {}

resource "klaviyo_profile" "sarah" {
  email        = "sarah.mason@example.com"
  phone_number = "+15005550006"
  first_name   = "Sarah"
  last_name    = "Mason"
  organization = "Example Corporation"
  locale       = "en-US"

  properties = jsonencode({
    tier      = "gold"
    favorites = ["candle", "lotion"]
  })
}
