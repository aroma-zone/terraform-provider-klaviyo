// Package resource_segment implements the klaviyo_segment managed
// resource. Like klaviyo_flow, segments are defined by a complex
// SegmentDefinition blob the API only accepts in full on Create. The
// definition can be replaced via PATCH (the API accepts it on Update
// alongside name/is_starred/is_active), but in practice the
// definition is built in Klaviyo's UI; we model it as a normalized
// JSON string for clean diffs.
package resource_segment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*segmentResource)(nil)
	_ resource.ResourceWithConfigure   = (*segmentResource)(nil)
	_ resource.ResourceWithImportState = (*segmentResource)(nil)
)

const apiType = "segment"

func New() resource.Resource {
	return &segmentResource{}
}

type segmentResource struct {
	c *client.Client
}

type model struct {
	ID           types.String         `tfsdk:"id"`
	Name         types.String         `tfsdk:"name"`
	Definition   jsontypes.Normalized `tfsdk:"definition"`
	IsStarred    types.Bool           `tfsdk:"is_starred"`
	IsActive     types.Bool           `tfsdk:"is_active"`
	IsProcessing types.Bool           `tfsdk:"is_processing"`
	Created      types.String         `tfsdk:"created"`
	Updated      types.String         `tfsdk:"updated"`
}

type envelope struct {
	Data resourceObject `json:"data"`
}

type resourceObject struct {
	Type       string     `json:"type"`
	ID         string     `json:"id,omitempty"`
	Attributes attributes `json:"attributes"`
}

type attributes struct {
	Name         string          `json:"name,omitempty"`
	Definition   json.RawMessage `json:"definition,omitempty"`
	IsStarred    *bool           `json:"is_starred,omitempty"`
	IsActive     *bool           `json:"is_active,omitempty"`
	IsProcessing *bool           `json:"is_processing,omitempty"`
	Created      *string         `json:"created,omitempty"`
	Updated      *string         `json:"updated,omitempty"`
}

func (r *segmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_segment"
}

func (r *segmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Klaviyo audience segment. The `definition` attribute carries the full SegmentDefinition JSON blob; build the segment in Klaviyo's UI, export it, and embed via `file()` or `jsonencode()`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Klaviyo-assigned identifier.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Human-readable segment name.",
				Required:    true,
			},
			"definition": schema.StringAttribute{
				Description: "SegmentDefinition JSON. Whitespace-only differences do not produce diffs.",
				Required:    true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"is_starred": schema.BoolAttribute{
				Description: "Whether the segment is starred (highlighted in the Klaviyo UI). Defaults to false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"is_active": schema.BoolAttribute{
				Description: "Whether the segment is active. Setting to false deactivates the segment.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"is_processing": schema.BoolAttribute{
				Description: "Server-assigned: true while Klaviyo is recomputing the segment membership.",
				Computed:    true,
			},
			"created": schema.StringAttribute{
				Description: "ISO-8601 timestamp the segment was created (server-assigned).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated": schema.StringAttribute{
				Description: "ISO-8601 timestamp the segment was last updated (server-assigned).",
				Computed:    true,
			},
		},
	}
}

func (r *segmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *segmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan model
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	body := envelope{Data: resourceObject{
		Type:       apiType,
		Attributes: attributesFromPlan(plan),
	}}
	out, err := r.send(ctx, http.MethodPost, "/api/segments/", body, http.StatusCreated)
	if err != nil {
		resp.Diagnostics.AddError("Creating Klaviyo segment failed", err.Error())
		return
	}
	mergeIntoModel(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *segmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state model
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	out, err := r.send(ctx, http.MethodGet, "/api/segments/"+state.ID.ValueString(), nil, http.StatusOK)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading Klaviyo segment failed", err.Error())
		return
	}
	mergeIntoModel(&state, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *segmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
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
	out, err := r.send(ctx, http.MethodPatch, "/api/segments/"+plan.ID.ValueString(), body, http.StatusOK)
	if err != nil {
		resp.Diagnostics.AddError("Updating Klaviyo segment failed", err.Error())
		return
	}
	mergeIntoModel(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *segmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state model
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	httpResp, err := r.c.Do(ctx, http.MethodDelete, "/api/segments/"+state.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Deleting Klaviyo segment failed", err.Error())
		return
	}
	defer httpResp.Body.Close()
	switch httpResp.StatusCode {
	case http.StatusNoContent, http.StatusOK, http.StatusNotFound:
		return
	default:
		resp.Diagnostics.AddError("Deleting Klaviyo segment failed", client.DecodeError(httpResp).Error())
	}
}

func (r *segmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *segmentResource) send(ctx context.Context, method, urlPath string, body any, expectedStatus int) (resourceObject, error) {
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

func attributesFromPlan(m model) attributes {
	a := attributes{
		Name:       m.Name.ValueString(),
		Definition: json.RawMessage(m.Definition.ValueString()),
	}
	if !m.IsStarred.IsUnknown() && !m.IsStarred.IsNull() {
		v := m.IsStarred.ValueBool()
		a.IsStarred = &v
	}
	if !m.IsActive.IsUnknown() && !m.IsActive.IsNull() {
		v := m.IsActive.ValueBool()
		a.IsActive = &v
	}
	return a
}

func mergeIntoModel(m *model, r resourceObject) {
	m.ID = types.StringValue(r.ID)
	if r.Attributes.Name != "" {
		m.Name = types.StringValue(r.Attributes.Name)
	}
	// Server returns the definition; let it overwrite the plan so
	// subsequent reads see the canonical normalized form.
	if len(r.Attributes.Definition) > 0 {
		m.Definition = jsontypes.NewNormalizedValue(string(r.Attributes.Definition))
	}
	if r.Attributes.IsStarred != nil {
		m.IsStarred = types.BoolValue(*r.Attributes.IsStarred)
	}
	if r.Attributes.IsActive != nil {
		m.IsActive = types.BoolValue(*r.Attributes.IsActive)
	}
	if r.Attributes.IsProcessing != nil {
		m.IsProcessing = types.BoolValue(*r.Attributes.IsProcessing)
	} else {
		m.IsProcessing = types.BoolNull()
	}
	if r.Attributes.Created != nil {
		m.Created = types.StringValue(*r.Attributes.Created)
	}
	if r.Attributes.Updated != nil {
		m.Updated = types.StringValue(*r.Attributes.Updated)
	}
}
