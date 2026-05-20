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

resource "klaviyo_coupon" "ten_off" {
  external_id = "10OFF"
  description = "10% off purchases over $50"

  monitor_configuration = {
    low_balance_threshold = 500
  }
}

output "coupon_id" {
  value = klaviyo_coupon.ten_off.id
}
