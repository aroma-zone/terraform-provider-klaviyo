#!/usr/bin/env bash
# Import an existing Klaviyo flow into Terraform state.
# Replace <flow-id> with the Klaviyo flow ID (e.g. "XVTP5Q").
# Note: the `definition` attribute is NOT populated by import —
# you must also write it into HCL to match the upstream flow.
terraform import klaviyo_flow.welcome <flow-id>
