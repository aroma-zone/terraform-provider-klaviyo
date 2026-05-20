// Package resource_list implements the klaviyo_list managed resource.
//
// Klaviyo exposes lists at /api/lists with a JSON:API envelope. This
// package flattens that envelope into idiomatic Terraform attributes so
// users write `resource "klaviyo_list" "x" { name = "..." }` rather
// than nesting under data/attributes blocks.
package resource_list

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time checks that listResource satisfies the Plugin
// Framework interfaces we expect to implement.
var (
	_ resource.Resource                = (*listResource)(nil)
	_ resource.ResourceWithConfigure   = (*listResource)(nil)
	_ resource.ResourceWithImportState = (*listResource)(nil)
)

// apiType is the JSON:API resource `type` value Klaviyo expects for
// lists.
const apiType = "list"

// New returns a constructor for the klaviyo_list resource, matching the
// signature provider.Resources() requires.
func New() resource.Resource {
	return &listResource{}
}

// listResource implements resource.Resource.
type listResource struct {
	c *client.Client
}

// model mirrors the resource block users write in HCL.
type model struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	OptInProcess types.String `tfsdk:"opt_in_process"`
	Created      types.String `tfsdk:"created"`
	Updated      types.String `tfsdk:"updated"`
}

// JSON:API DTOs. Kept private to this package because no other resource
// shares the shape.

type envelope struct {
	Data resourceObject `json:"data"`
}

type resourceObject struct {
	Type       string     `json:"type"`
	ID         string     `json:"id,omitempty"`
	Attributes attributes `json:"attributes"`
}

type attributes struct {
	Name         string  `json:"name,omitempty"`
	OptInProcess *string `json:"opt_in_process,omitempty"`
	Created      string  `json:"created,omitempty"`
	Updated      string  `json:"updated,omitempty"`
}

func (r *listResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_list"
}

func (r *listResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Klaviyo audience list (https://developers.klaviyo.com/en/reference/lists).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Klaviyo-assigned identifier for the list.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Human-readable name for the list.",
				Required:    true,
			},
			"opt_in_process": schema.StringAttribute{
				Description: "Opt-in process for new subscribers. One of `double_opt_in` or `single_opt_in`. Defaults to the Klaviyo account default if omitted.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("double_opt_in", "single_opt_in"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created": schema.StringAttribute{
				Description: "ISO-8601 timestamp the list was created (server-assigned).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated": schema.StringAttribute{
				Description: "ISO-8601 timestamp the list was last updated (server-assigned).",
				Computed:    true,
			},
		},
	}
}

// Configure picks up the *client.Client the provider built in its own
// Configure. Called once per resource instance.
func (r *listResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		// During the initial validation phase ProviderData is nil. That's
		// expected; we'll be called again with real data later.
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

func (r *listResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan model
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	body := envelope{Data: resourceObject{
		Type:       apiType,
		Attributes: attributesFromPlan(plan),
	}}
	out, err := r.send(ctx, http.MethodPost, "/api/lists/", body, http.StatusCreated)
	if err != nil {
		resp.Diagnostics.AddError("Creating Klaviyo list failed", err.Error())
		return
	}
	mergeIntoModel(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *listResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state model
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	out, err := r.send(ctx, http.MethodGet, "/api/lists/"+state.ID.ValueString(), nil, http.StatusOK)
	if err != nil {
		if client.IsNotFound(err) {
			// Resource disappeared upstream; drop it from state so the
			// next plan recreates it.
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading Klaviyo list failed", err.Error())
		return
	}
	mergeIntoModel(&state, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *listResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan model
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	body := envelope{Data: resourceObject{
		Type:       apiType,
		ID:         plan.ID.ValueString(),
		Attributes: attributesFromPlan(plan),
	}}
	out, err := r.send(ctx, http.MethodPatch, "/api/lists/"+plan.ID.ValueString(), body, http.StatusOK)
	if err != nil {
		resp.Diagnostics.AddError("Updating Klaviyo list failed", err.Error())
		return
	}
	mergeIntoModel(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *listResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state model
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	resp2, err := r.c.Do(ctx, http.MethodDelete, "/api/lists/"+state.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Deleting Klaviyo list failed", err.Error())
		return
	}
	defer resp2.Body.Close()
	switch resp2.StatusCode {
	case http.StatusNoContent, http.StatusOK, http.StatusNotFound:
		// Already gone is fine — Terraform's job is to make the resource
		// not exist, and it doesn't.
		return
	default:
		resp.Diagnostics.AddError("Deleting Klaviyo list failed", client.DecodeError(resp2).Error())
	}
}

// ImportState lets users `terraform import klaviyo_list.x <id>`.
func (r *listResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// send marshals body, performs the request, and (on a successful
// expectedStatus) decodes the response envelope. On any other status it
// returns the decoded APIError.
func (r *listResource) send(ctx context.Context, method, urlPath string, body any, expectedStatus int) (resourceObject, error) {
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

// attributesFromPlan converts a plan model into the JSON:API attributes
// block we send to Klaviyo. opt_in_process is omitted when the user
// didn't set it; with `omitempty + *string`, that translates to "don't
// touch this field" on both POST and PATCH.
func attributesFromPlan(m model) attributes {
	a := attributes{Name: m.Name.ValueString()}
	if !m.OptInProcess.IsUnknown() && !m.OptInProcess.IsNull() {
		v := m.OptInProcess.ValueString()
		a.OptInProcess = &v
	}
	return a
}

// mergeIntoModel overwrites every server-owned field with the values
// the API returned. The server is authoritative for all of these
// (including name and opt_in_process — the user's input is reflected
// back).
func mergeIntoModel(m *model, r resourceObject) {
	m.ID = types.StringValue(r.ID)
	m.Name = types.StringValue(r.Attributes.Name)
	if r.Attributes.OptInProcess != nil {
		m.OptInProcess = types.StringValue(*r.Attributes.OptInProcess)
	} else {
		m.OptInProcess = types.StringNull()
	}
	m.Created = types.StringValue(r.Attributes.Created)
	m.Updated = types.StringValue(r.Attributes.Updated)
}
