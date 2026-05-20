terraform {
  required_providers {
    klaviyo = { source = "aroma-zone/klaviyo", version = "~> 0.1" }
  }
}
provider "klaviyo" {}

resource "klaviyo_segment" "vips" {
  name       = "VIP Customers"
  definition = file("${path.module}/vip_segment.json")
  is_starred = true
}
