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
  # api_revision defaults to the revision this provider was built for.
}

resource "klaviyo_list" "newsletter" {
  name           = "Newsletter"
  opt_in_process = "single_opt_in"
}

output "newsletter_id" {
  value = klaviyo_list.newsletter.id
}
