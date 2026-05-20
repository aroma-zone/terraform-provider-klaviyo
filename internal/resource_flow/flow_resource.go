// Package resource_flow implements the klaviyo_flow managed resource.
//
// Klaviyo flows are large, declarative workflow definitions (triggers,
// branches, messages). Their Create API accepts the full
// `FlowDefinition` blob as a single JSON object; their Update API only
// accepts `status`. To stay honest to that asymmetry, this resource
// models `definition` as a JSON-typed string (jsontypes.Normalized) so
// HCL whitespace differences don't produce false diffs, marks
// `name`/`definition` RequiresReplace, and lets `status` round-trip
// via PATCH.
//
// Typical workflow:
//   1. Build the flow in Klaviyo's UI.
//   2. Export the definition JSON.
//   3. Embed it in HCL via `definition = file("flow.json")`
//      or `definition = jsonencode({...})`.
package resource_flow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*flowResource)(nil)
	_ resource.ResourceWithConfigure   = (*flowResource)(nil)
	_ resource.ResourceWithImportState = (*flowResource)(nil)
)

const apiType = "flow"

// New is the constructor used by provider.Resources().
func New() resource.Resource {
	return &flowResource{}
}

type flowResource struct {
	c *client.Client
}

// model mirrors the HCL block.
type model struct {
	ID          types.String         `tfsdk:"id"`
	Name        types.String         `tfsdk:"name"`
	Definition  jsontypes.Normalized `tfsdk:"definition"`
	Status      types.String         `tfsdk:"status"`
	Archived    types.Bool           `tfsdk:"archived"`
	TriggerType types.String         `tfsdk:"trigger_type"`
	Created     types.String         `tfsdk:"created"`
	Updated     types.String         `tfsdk:"updated"`
}

// JSON:API DTOs.

type envelope struct {
	Data resourceObject `json:"data"`
}

type resourceObject struct {
	Type       string     `json:"type"`
	ID         string     `json:"id,omitempty"`
	Attributes attributes `json:"attributes"`
}

// attributes covers everything the API can return.
//
// `Definition` is the FlowDefinition blob the API expects on POST;
// it's never returned on Read (the server replies with the assembled
// shape — name/status/etc.). We unmarshal it into json.RawMessage to
// stay opaque on the way in.
type attributes struct {
	Name        string          `json:"name,omitempty"`
	Definition  json.RawMessage `json:"definition,omitempty"`
	Status      string          `json:"status,omitempty"`
	Archived    *bool           `json:"archived,omitempty"`
	TriggerType *string         `json:"trigger_type,omitempty"`
	Created     *string         `json:"created,omitempty"`
	Updated     *string         `json:"updated,omitempty"`
}

func (r *flowResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_flow"
}

func (r *flowResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}

	resp.Schema = schema.Schema{
		Description: "A Klaviyo automation flow. Klaviyo only allows updating `status` via the API — changing `name` or `definition` forces destroy + recreate. The `definition` field carries the full FlowDefinition JSON blob; build the flow in Klaviyo's UI, export the JSON, and embed it here.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Klaviyo identifier (e.g. `XVTP5Q`).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description:   "Flow name displayed in Klaviyo.",
				Required:      true,
				PlanModifiers: requiresReplace,
			},
			"definition": schema.StringAttribute{
				Description: "FlowDefinition JSON. Best practice: keep this in a separate `.json` file and load via `file(\"flow.json\")` so the definition is reviewable on its own. Whitespace differences do not produce diffs (jsontypes.Normalized).",
				Required:    true,
				CustomType:  jsontypes.NormalizedType{},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"status": schema.StringAttribute{
				Description: "Lifecycle state: `draft`, `manual`, or `live`. Default `draft`. Updatable in place.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("draft"),
				Validators: []validator.String{
					stringvalidator.OneOf("draft", "manual", "live"),
				},
			},
			"archived": schema.BoolAttribute{
				Description: "Whether the flow is archived (server-assigned).",
				Computed:    true,
			},
			"trigger_type": schema.StringAttribute{
				Description: "Computed by Klaviyo based on the flow definition. One of `Added to List`, `Date Based`, `Low Inventory`, `Metric`, `Price Drop`, `Unconfigured`.",
				Computed:    true,
			},
			"created": schema.StringAttribute{
				Description: "ISO-8601 timestamp the flow was created (server-assigned).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated": schema.StringAttribute{
				Description: "ISO-8601 timestamp the flow was last updated (server-assigned).",
				Computed:    true,
			},
		},
	}
}

func (r *flowResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client, got %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}
	r.c = c
}

func (r *flowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan model
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	body := envelope{Data: resourceObject{
		Type: apiType,
		Attributes: attributes{
			Name:       plan.Name.ValueString(),
			Definition: json.RawMessage(plan.Definition.ValueString()),
		},
	}}
	out, err := r.send(ctx, http.MethodPost, "/api/flows/", body, http.StatusCreated)
	if err != nil {
		resp.Diagnostics.AddError("Creating Klaviyo flow failed", err.Error())
		return
	}
	mergeIntoModel(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *flowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state model
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	out, err := r.send(ctx, http.MethodGet, "/api/flows/"+state.ID.ValueString(), nil, http.StatusOK)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading Klaviyo flow failed", err.Error())
		return
	}
	mergeIntoModel(&state, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update is status-only by design (the API only accepts `status` on
// PATCH). All other writable attributes carry RequiresReplace, so any
// other change goes through destroy + recreate.
func (r *flowResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan model
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	body := envelope{Data: resourceObject{
		Type:       apiType,
		ID:         plan.ID.ValueString(),
		Attributes: attributes{Status: plan.Status.ValueString()},
	}}
	out, err := r.send(ctx, http.MethodPatch, "/api/flows/"+plan.ID.ValueString(), body, http.StatusOK)
	if err != nil {
		resp.Diagnostics.AddError("Updating Klaviyo flow failed", err.Error())
		return
	}
	mergeIntoModel(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *flowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state model
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	httpResp, err := r.c.Do(ctx, http.MethodDelete, "/api/flows/"+state.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Deleting Klaviyo flow failed", err.Error())
		return
	}
	defer httpResp.Body.Close()
	switch httpResp.StatusCode {
	case http.StatusNoContent, http.StatusOK, http.StatusNotFound:
		return
	default:
		resp.Diagnostics.AddError("Deleting Klaviyo flow failed", client.DecodeError(httpResp).Error())
	}
}

func (r *flowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *flowResource) send(ctx context.Context, method, urlPath string, body any, expectedStatus int) (resourceObject, error) {
	httpResp, err := r.c.Do(ctx, method, urlPath, body)
	if err != nil {
		return resourceObject{}, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != expectedStatus {
		return resourceObject{}, client.DecodeError(httpResp)
	}
	if httpResp.StatusCode == http.StatusNoContent {
		return resourceObject{}, nil
	}
	var env envelope
	if err := json.NewDecoder(httpResp.Body).Decode(&env); err != nil {
		return resourceObject{}, fmt.Errorf("decoding Klaviyo response: %w", err)
	}
	return env.Data, nil
}

// mergeIntoModel copies server-owned fields into the Terraform state.
// `definition` is NOT touched here — the API does not return it on
// read, so we leave whatever the plan/state already has in place.
func mergeIntoModel(m *model, r resourceObject) {
	m.ID = types.StringValue(r.ID)
	if r.Attributes.Name != "" {
		m.Name = types.StringValue(r.Attributes.Name)
	}
	if r.Attributes.Status != "" {
		m.Status = types.StringValue(r.Attributes.Status)
	}
	if r.Attributes.Archived != nil {
		m.Archived = types.BoolValue(*r.Attributes.Archived)
	} else {
		m.Archived = types.BoolNull()
	}
	if r.Attributes.TriggerType != nil {
		m.TriggerType = types.StringValue(*r.Attributes.TriggerType)
	} else {
		m.TriggerType = types.StringNull()
	}
	if r.Attributes.Created != nil {
		m.Created = types.StringValue(*r.Attributes.Created)
	}
	if r.Attributes.Updated != nil {
		m.Updated = types.StringValue(*r.Attributes.Updated)
	}
}
