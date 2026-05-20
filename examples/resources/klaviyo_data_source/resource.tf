terraform {
  required_providers {
    klaviyo = {
      source  = "aroma-zone/klaviyo"
      version = "~> 0.1"
    }
  }
}

provider "klaviyo" {
  # api_key is read from KLAVIYO_API_KEY if not set here.
}

# Klaviyo data sources are immutable: any change to title, visibility,
# description, or namespace triggers a destroy + recreate.
resource "klaviyo_data_source" "warehouse" {
  title       = "Warehouse"
  visibility  = "private"
  description = "Source for warehouse-imported events"
  # namespace defaults to "custom-objects"
}

output "data_source_id" {
  value = klaviyo_data_source.warehouse.id
}
