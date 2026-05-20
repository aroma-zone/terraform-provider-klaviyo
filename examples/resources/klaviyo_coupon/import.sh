#!/usr/bin/env bash
# Import an existing Klaviyo coupon into Terraform state.
# The Klaviyo coupon id equals its external_id (e.g. "10OFF").
terraform import klaviyo_coupon.ten_off <external-id>
