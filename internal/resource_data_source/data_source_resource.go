// Package resource_data_source implements the klaviyo_data_source
// managed resource. Klaviyo data sources sit under the Custom Objects
// surface and identify the system that fed data into Klaviyo.
//
// Klaviyo does not expose a PATCH on this resource: any change to a
// writable attribute forces Terraform to destroy and recreate it, so
// every config-writable attribute carries the RequiresReplace plan
// modifier.
package resource_data_source

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*dataSourceResource)(nil)
	_ resource.ResourceWithConfigure   = (*dataSourceResource)(nil)
	_ resource.ResourceWithImportState = (*dataSourceResource)(nil)
)

// apiType is the JSON:API resource `type` value Klaviyo expects.
const apiType = "data-source"

// New returns a constructor matching provider.Resources()'s signature.
func New() resource.Resource {
	return &dataSourceResource{}
}

// dataSourceResource implements resource.Resource for klaviyo_data_source.
type dataSourceResource struct {
	c *client.Client
}

// model mirrors the HCL block users write.
type model struct {
	ID          types.String `tfsdk:"id"`
	Title       types.String `tfsdk:"title"`
	Visibility  types.String `tfsdk:"visibility"`
	Description types.String `tfsdk:"description"`
	Namespace   types.String `tfsdk:"namespace"`
}

// JSON:API DTOs, private to this package.

type envelope struct {
	Data resourceObject `json:"data"`
}

type resourceObject struct {
	Type       string     `json:"type"`
	ID         string     `json:"id,omitempty"`
	Attributes attributes `json:"attributes"`
}

type attributes struct {
	Title       string  `json:"title"`
	Visibility  *string `json:"visibility,omitempty"`
	Description *string `json:"description,omitempty"`
	Namespace   *string `json:"namespace,omitempty"`
}

func (r *dataSourceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_data_source"
}

func (r *dataSourceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}

	resp.Schema = schema.Schema{
		Description: "A Klaviyo data source (Custom Objects). Any change to a configurable attribute forces a destroy + recreate, because Klaviyo's API does not expose a PATCH on this resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Klaviyo-assigned identifier for the data source.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"title": schema.StringAttribute{
				Description:   "Title of the data source. Must be unique within the namespace.",
				Required:      true,
				PlanModifiers: requiresReplace,
			},
			"visibility": schema.StringAttribute{
				Description: "`private` (default) or `shared`. Controls whether the data source is shared across the account.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("private"),
				Validators: []validator.String{
					stringvalidator.OneOf("private", "shared"),
				},
				PlanModifiers: requiresReplace,
			},
			"description": schema.StringAttribute{
				Description:   "Human-readable description.",
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString(""),
				PlanModifiers: requiresReplace,
			},
			"namespace": schema.StringAttribute{
				Description:   "Logical namespace the data source belongs to (defaults to `custom-objects`).",
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString("custom-objects"),
				PlanModifiers: requiresReplace,
			},
		},
	}
}

func (r *dataSourceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *dataSourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan model
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	body := envelope{Data: resourceObject{
		Type:       apiType,
		Attributes: attributesFromPlan(plan),
	}}
	out, err := r.send(ctx, http.MethodPost, "/api/data-sources/", body, http.StatusCreated)
	if err != nil {
		resp.Diagnostics.AddError("Creating Klaviyo data source failed", err.Error())
		return
	}
	mergeIntoModel(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *dataSourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state model
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	out, err := r.send(ctx, http.MethodGet, "/api/data-sources/"+state.ID.ValueString(), nil, http.StatusOK)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading Klaviyo data source failed", err.Error())
		return
	}
	mergeIntoModel(&state, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update is a no-op: every writable attribute carries RequiresReplace,
// so Terraform never calls Update for an in-place change. Implemented
// only because resource.Resource requires it.
func (r *dataSourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan model
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *dataSourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state model
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	httpResp, err := r.c.Do(ctx, http.MethodDelete, "/api/data-sources/"+state.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Deleting Klaviyo data source failed", err.Error())
		return
	}
	defer httpResp.Body.Close()
	switch httpResp.StatusCode {
	case http.StatusNoContent, http.StatusOK, http.StatusNotFound:
		return
	default:
		resp.Diagnostics.AddError("Deleting Klaviyo data source failed", client.DecodeError(httpResp).Error())
	}
}

func (r *dataSourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// send is the same shape as resource_list's helper — see there for the
// rationale on returning the decoded envelope directly.
func (r *dataSourceResource) send(ctx context.Context, method, urlPath string, body any, expectedStatus int) (resourceObject, error) {
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

// attributesFromPlan turns a populated model into the JSON:API
// attributes block. Optional+Computed fields are sent when known and
// not null; omitted otherwise so server-side defaults apply.
func attributesFromPlan(m model) attributes {
	a := attributes{Title: m.Title.ValueString()}
	if knownString(m.Visibility) {
		v := m.Visibility.ValueString()
		a.Visibility = &v
	}
	if knownString(m.Description) {
		v := m.Description.ValueString()
		a.Description = &v
	}
	if knownString(m.Namespace) {
		v := m.Namespace.ValueString()
		a.Namespace = &v
	}
	return a
}

func knownString(s types.String) bool { return !s.IsUnknown() && !s.IsNull() }

func mergeIntoModel(m *model, r resourceObject) {
	m.ID = types.StringValue(r.ID)
	m.Title = types.StringValue(r.Attributes.Title)
	if r.Attributes.Visibility != nil {
		m.Visibility = types.StringValue(*r.Attributes.Visibility)
	} else {
		m.Visibility = types.StringNull()
	}
	if r.Attributes.Description != nil {
		m.Description = types.StringValue(*r.Attributes.Description)
	} else {
		m.Description = types.StringNull()
	}
	if r.Attributes.Namespace != nil {
		m.Namespace = types.StringValue(*r.Attributes.Namespace)
	} else {
		m.Namespace = types.StringNull()
	}
}
