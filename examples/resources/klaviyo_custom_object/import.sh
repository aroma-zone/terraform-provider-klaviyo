#!/usr/bin/env bash
# Import an existing Klaviyo Custom Object (object schema) into state.
# Replace <id> with the ULID Klaviyo assigned (e.g. "01K61DKJ7EKJ0ES9VE456XF5JG").
terraform import klaviyo_custom_object.person <id>
