terraform {
  required_providers {
    klaviyo = {
      source  = "aroma-zone/klaviyo"
      version = "~> 0.1"
    }
  }
}

provider "klaviyo" {}

# Keep the FlowDefinition JSON in a separate file. Build the flow in
# Klaviyo's UI, export the definition, and check it in alongside the
# Terraform config.
resource "klaviyo_flow" "welcome" {
  name       = "Welcome Series"
  definition = file("${path.module}/welcome_flow.json")
  status     = "draft"
}

output "flow_id" {
  value = klaviyo_flow.welcome.id
}
