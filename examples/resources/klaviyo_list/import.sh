#!/usr/bin/env bash
# Import an existing Klaviyo list into Terraform state.
# Replace <list-id> with the Klaviyo list ID (e.g. "Y6nRLr").
terraform import klaviyo_list.newsletter <list-id>
