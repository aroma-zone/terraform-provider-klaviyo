terraform {
  required_providers {
    klaviyo = {
      source  = "aroma-zone/klaviyo"
      version = "~> 0.1"
    }
  }
}

provider "klaviyo" {
  # api_revision defaults to 2026-04-15.pre, which is required to call
  # the Custom Objects beta endpoint that backs this resource.
}

resource "klaviyo_custom_object" "person" {
  title       = "Person"
  description = "A customer-like entity from an external warehouse."
  status      = "DRAFT"
  required    = ["name"]

  properties = [
    {
      id   = 1
      name = "name"
      type = "STRING"
    },
    {
      id          = 2
      name        = "age"
      type        = "INT"
      description = "Age in years."
    },
  ]
}
