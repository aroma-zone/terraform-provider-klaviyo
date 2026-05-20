#!/usr/bin/env bash
# Import an existing Klaviyo data source into Terraform state.
# Replace <data-source-id> with the Klaviyo data source ID (e.g. "01KH1D6P9Y8TJ7Q6MHXWZMPDN3").
terraform import klaviyo_data_source.warehouse <data-source-id>
