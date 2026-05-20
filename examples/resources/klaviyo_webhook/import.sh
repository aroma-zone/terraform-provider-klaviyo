#!/usr/bin/env bash
# Import an existing Klaviyo webhook into Terraform state.
# After import you must add `endpoint_url` and `secret_key` to your HCL
# matching the upstream values — Klaviyo doesn't return either on read.
terraform import klaviyo_webhook.sms_events <webhook-id>
