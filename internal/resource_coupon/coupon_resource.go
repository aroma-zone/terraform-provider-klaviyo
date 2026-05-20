// Package resource_coupon implements the klaviyo_coupon managed
// resource — the parent coupon definition (not the individual coupon
// codes, which live under /api/coupon-codes).
//
// external_id is the user-supplied identifier that's stored in the
// upstream integration (e.g. Shopify). It doubles as Klaviyo's internal
// id, so the Terraform `id` attribute mirrors external_id.
package resource_coupon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*couponResource)(nil)
	_ resource.ResourceWithConfigure   = (*couponResource)(nil)
	_ resource.ResourceWithImportState = (*couponResource)(nil)
)

// apiType is the JSON:API resource `type` value Klaviyo expects.
const apiType = "coupon"

// New returns a constructor matching provider.Resources()'s signature.
func New() resource.Resource {
	return &couponResource{}
}

type couponResource struct {
	c *client.Client
}

// model mirrors the HCL block users write.
type model struct {
	ID                   types.String         `tfsdk:"id"`
	ExternalID           types.String         `tfsdk:"external_id"`
	Description          types.String         `tfsdk:"description"`
	MonitorConfiguration *monitorConfigBlock  `tfsdk:"monitor_configuration"`
}

// monitorConfigBlock matches the SingleNestedAttribute below.
type monitorConfigBlock struct {
	LowBalanceThreshold types.Int64 `tfsdk:"low_balance_threshold"`
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

type attributes struct {
	ExternalID           string                `json:"external_id,omitempty"`
	Description          *string               `json:"description,omitempty"`
	MonitorConfiguration *monitorConfiguration `json:"monitor_configuration,omitempty"`
}

type monitorConfiguration struct {
	LowBalanceThreshold *int64 `json:"low_balance_threshold,omitempty"`
}

func (r *couponResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_coupon"
}

func (r *couponResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Klaviyo coupon definition (the parent; individual coupon codes are managed separately).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Klaviyo identifier. Equal to `external_id`; populated on create.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"external_id": schema.StringAttribute{
				Description: "Identifier as stored in the upstream integration (Shopify, Magento, etc.). Becomes Klaviyo's internal id and is immutable.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Description: "Human-readable description of the coupon.",
				Optional:    true,
			},
			"monitor_configuration": schema.SingleNestedAttribute{
				Description: "Optional monitor settings for the coupon.",
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"low_balance_threshold": schema.Int64Attribute{
						Description: "Alert when the unredeemed code balance drops below this number.",
						Optional:    true,
					},
				},
			},
		},
	}
}

func (r *couponResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *couponResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan model
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	body := envelope{Data: resourceObject{
		Type:       apiType,
		Attributes: attributesFromPlan(plan, true /* includeExternalID */),
	}}
	out, err := r.send(ctx, http.MethodPost, "/api/coupons/", body, http.StatusCreated)
	if err != nil {
		resp.Diagnostics.AddError("Creating Klaviyo coupon failed", err.Error())
		return
	}
	mergeIntoModel(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *couponResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state model
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	out, err := r.send(ctx, http.MethodGet, "/api/coupons/"+state.ID.ValueString(), nil, http.StatusOK)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading Klaviyo coupon failed", err.Error())
		return
	}
	mergeIntoModel(&state, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *couponResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan model
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	body := envelope{Data: resourceObject{
		Type:       apiType,
		ID:         plan.ID.ValueString(),
		Attributes: attributesFromPlan(plan, false),
	}}
	out, err := r.send(ctx, http.MethodPatch, "/api/coupons/"+plan.ID.ValueString(), body, http.StatusOK)
	if err != nil {
		resp.Diagnostics.AddError("Updating Klaviyo coupon failed", err.Error())
		return
	}
	mergeIntoModel(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *couponResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state model
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	httpResp, err := r.c.Do(ctx, http.MethodDelete, "/api/coupons/"+state.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Deleting Klaviyo coupon failed", err.Error())
		return
	}
	defer httpResp.Body.Close()
	switch httpResp.StatusCode {
	case http.StatusNoContent, http.StatusOK, http.StatusNotFound:
		return
	default:
		resp.Diagnostics.AddError("Deleting Klaviyo coupon failed", client.DecodeError(httpResp).Error())
	}
}

func (r *couponResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by id == external_id. We need to set BOTH id and
	// external_id so subsequent plans don't see external_id as unknown.
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.State.SetAttribute(ctx, path.Root("external_id"), req.ID)
}

func (r *couponResource) send(ctx context.Context, method, urlPath string, body any, expectedStatus int) (resourceObject, error) {
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

// attributesFromPlan builds the wire-format attributes block. When
// includeExternalID is false (update path) we leave external_id out
// because Klaviyo's PATCH endpoint doesn't accept it.
func attributesFromPlan(m model, includeExternalID bool) attributes {
	a := attributes{}
	if includeExternalID {
		a.ExternalID = m.ExternalID.ValueString()
	}
	if !m.Description.IsUnknown() && !m.Description.IsNull() {
		v := m.Description.ValueString()
		a.Description = &v
	}
	if m.MonitorConfiguration != nil && !m.MonitorConfiguration.LowBalanceThreshold.IsUnknown() && !m.MonitorConfiguration.LowBalanceThreshold.IsNull() {
		thr := m.MonitorConfiguration.LowBalanceThreshold.ValueInt64()
		a.MonitorConfiguration = &monitorConfiguration{LowBalanceThreshold: &thr}
	}
	return a
}

func mergeIntoModel(m *model, r resourceObject) {
	m.ID = types.StringValue(r.ID)
	m.ExternalID = types.StringValue(r.Attributes.ExternalID)
	if r.Attributes.Description != nil {
		m.Description = types.StringValue(*r.Attributes.Description)
	} else {
		m.Description = types.StringNull()
	}
	if r.Attributes.MonitorConfiguration != nil {
		block := &monitorConfigBlock{}
		if r.Attributes.MonitorConfiguration.LowBalanceThreshold != nil {
			block.LowBalanceThreshold = types.Int64Value(*r.Attributes.MonitorConfiguration.LowBalanceThreshold)
		} else {
			block.LowBalanceThreshold = types.Int64Null()
		}
		m.MonitorConfiguration = block
	} else {
		m.MonitorConfiguration = nil
	}
}
