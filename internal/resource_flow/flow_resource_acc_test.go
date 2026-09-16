// Acceptance tests for klaviyo_flow, run by Terraform CLI against the
// real Klaviyo API. Enable with TF_ACC=1 and a valid KLAVIYO_API_KEY:
//
//	make testacc TESTARGS='-run TestAccFlow'
//
// This file is package resource_flow_test (external) on purpose: the
// harness in internal/acctest imports internal/provider, which imports
// this package, so an internal test file here would be an import cycle.
//
// Safety: every flow these tests create is triggered by a list the test
// itself creates (so it is empty, and nothing is enqueued) and contains
// a single time-delay action. There is no send-email/send-sms action
// anywhere in this file, and no test ever sets status = "live". Running
// this suite cannot deliver a message to a real customer.
package resource_flow_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/acctest"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const flowResourceName = "klaviyo_flow.test"

// config renders a list + flow pair. The flow's trigger points at the
// list created in the same config, which is what makes this safe to run
// against a live account: the list is brand new and empty.
func config(listName, flowName, status string, delayDays int) string {
	return fmt.Sprintf(`
resource "klaviyo_list" "test" {
  name = %[1]q
}

resource "klaviyo_flow" "test" {
  name   = %[2]q
  status = %[3]q

  definition = jsonencode({
    triggers = [{
      type = "list"
      id   = klaviyo_list.test.id
    }]
    entry_action_id = "delay-1"
    actions = [{
      type         = "time-delay"
      temporary_id = "delay-1"
      data = {
        unit  = "days"
        value = %[4]d
      }
    }]
  })
}
`, listName, flowName, status, delayDays)
}

// checkExistsInKlaviyo proves the flow is really in Klaviyo — not just
// in Terraform state — and that the server agrees about name and
// status. This is the assertion that would catch a Create that wrote
// state without ever reaching the API.
func checkExistsInKlaviyo(wantName, wantStatus string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := flowIDFromState(s)
		if err != nil {
			return err
		}
		c, err := acctest.APIClient()
		if err != nil {
			return err
		}
		status, attrs, err := acctest.GetAttributes(c, "/api/flows/"+id)
		if err != nil {
			return err
		}
		if status == http.StatusNotFound {
			return fmt.Errorf("flow %s is in Terraform state but returns 404 from Klaviyo", id)
		}
		if got := attrs["name"]; got != wantName {
			return fmt.Errorf("Klaviyo reports name = %v, want %q", got, wantName)
		}
		if got := attrs["status"]; got != wantStatus {
			return fmt.Errorf("Klaviyo reports status = %v, want %q", got, wantStatus)
		}
		return nil
	}
}

// flowIDFromState pulls the flow's Klaviyo id out of Terraform state.
func flowIDFromState(s *terraform.State) (string, error) {
	rs, ok := s.RootModule().Resources[flowResourceName]
	if !ok {
		return "", fmt.Errorf("%s not found in state", flowResourceName)
	}
	if rs.Primary.ID == "" {
		return "", fmt.Errorf("%s has an empty id in state", flowResourceName)
	}
	return rs.Primary.ID, nil
}

// deleteFlowOutOfBand deletes the flow behind Terraform's back, the way
// someone clicking "delete" in the Klaviyo UI would.
func deleteFlowOutOfBand() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := flowIDFromState(s)
		if err != nil {
			return err
		}
		c, err := acctest.APIClient()
		if err != nil {
			return err
		}
		return acctest.DeleteOutOfBand(c, "/api/flows/"+id)
	}
}

// checkDestroyed asserts every flow the test created is gone from
// Klaviyo after `terraform destroy`.
func checkDestroyed(s *terraform.State) error {
	c, err := acctest.APIClient()
	if err != nil {
		return err
	}
	for name, rs := range s.RootModule().Resources {
		if rs.Type != "klaviyo_flow" {
			continue
		}
		gone, err := acctest.Deleted(c, "/api/flows/"+rs.Primary.ID)
		if err != nil {
			return err
		}
		if !gone {
			return fmt.Errorf("%s (%s) still exists in Klaviyo after destroy", name, rs.Primary.ID)
		}
	}
	return nil
}

// TestAccFlow_create is the core case: a real Create against Klaviyo,
// asserting both the Terraform state and the server-side object, then a
// clean destroy.
func TestAccFlow_create(t *testing.T) {
	listName := acctest.UniqueName("flow-create-list")
	flowName := acctest.UniqueName("flow-create")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:             checkDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config(listName, flowName, "draft", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Server-assigned identity.
					resource.TestCheckResourceAttrSet(flowResourceName, "id"),
					// Values we asked for.
					resource.TestCheckResourceAttr(flowResourceName, "name", flowName),
					resource.TestCheckResourceAttr(flowResourceName, "status", "draft"),
					// Values Klaviyo computes from the definition. A
					// list trigger must surface as "Added to List" —
					// this is what proves the definition blob was
					// accepted as an object, not as a string-of-JSON.
					resource.TestCheckResourceAttr(flowResourceName, "trigger_type", "Added to List"),
					resource.TestCheckResourceAttr(flowResourceName, "archived", "false"),
					// Computed timestamps must be known after apply.
					resource.TestCheckResourceAttrSet(flowResourceName, "created"),
					resource.TestCheckResourceAttrSet(flowResourceName, "updated"),
					// And it must actually exist upstream.
					checkExistsInKlaviyo(flowName, "draft"),
				),
			},
			{
				// A second plan on the unchanged config must be empty.
				// This is the regression guard for the jsontypes
				// round-trip: if `definition` did not normalize, or if
				// Read clobbered it, this step fails with a perpetual
				// diff.
				Config:   config(listName, flowName, "draft", 1),
				PlanOnly: true,
			},
		},
	})
}

// TestAccFlow_importState covers `terraform import`.
//
// Three attributes are excluded from the verify comparison, each
// pinning a real Klaviyo API behaviour rather than papering over a
// provider bug:
//
//   - definition: the read endpoint never returns it, which is why
//     mergeIntoModel deliberately leaves the field alone.
//   - created/updated: POST /api/flows/ returns sub-second precision
//     ("2026-09-16T10:02:02.127735+00:00") while GET /api/flows/{id}
//     truncates to the second ("2026-09-16T10:02:02+00:00"), so the
//     value written by Create can never equal the one written by Read.
//     Both are Computed-only, so the difference produces no plan diff
//     (TestAccFlow_create's PlanOnly step proves that) and the provider
//     stores what the API returned instead of inventing a normalised
//     form the API never sent.
//
// id, name, status, archived and trigger_type are still compared, so
// the import path is genuinely verified.
func TestAccFlow_importState(t *testing.T) {
	listName := acctest.UniqueName("flow-import-list")
	flowName := acctest.UniqueName("flow-import")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:             checkDestroyed,
		Steps: []resource.TestStep{
			{Config: config(listName, flowName, "draft", 1)},
			{
				Config:                  config(listName, flowName, "draft", 1),
				ResourceName:            flowResourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"definition", "created", "updated"},
			},
		},
	})
}

// TestAccFlow_definitionChangeForcesReplacement pins the RequiresReplace
// contract: Klaviyo's PATCH only accepts `status`, so any definition
// change must destroy and recreate rather than silently no-op.
func TestAccFlow_definitionChangeForcesReplacement(t *testing.T) {
	listName := acctest.UniqueName("flow-replace-list")
	flowName := acctest.UniqueName("flow-replace")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:             checkDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config(listName, flowName, "draft", 1),
				Check:  checkExistsInKlaviyo(flowName, "draft"),
			},
			{
				// Same name, different delay -> new definition.
				Config: config(listName, flowName, "draft", 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(flowResourceName, plancheck.ResourceActionReplace),
					},
				},
				Check: checkExistsInKlaviyo(flowName, "draft"),
			},
		},
	})
}

// TestAccFlow_disappears covers drift: when the flow is deleted in the
// Klaviyo UI, the next refresh must drop it from state and plan a
// recreate instead of erroring.
func TestAccFlow_disappears(t *testing.T) {
	listName := acctest.UniqueName("flow-gone-list")
	flowName := acctest.UniqueName("flow-gone")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:             checkDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config(listName, flowName, "draft", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkExistsInKlaviyo(flowName, "draft"),
					deleteFlowOutOfBand(),
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
