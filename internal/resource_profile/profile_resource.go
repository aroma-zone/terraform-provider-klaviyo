// Package resource_profile implements the klaviyo_profile managed
// resource. Klaviyo does not expose a DELETE for profiles — data
// removal is governed by the separate GDPR/data-privacy surface
// (suppressions). Terraform destroy therefore removes the profile
// from state and emits a warning pointing the operator at Klaviyo's
// data-privacy tooling.
package resource_profile

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*profileResource)(nil)
	_ resource.ResourceWithConfigure   = (*profileResource)(nil)
	_ resource.ResourceWithImportState = (*profileResource)(nil)
)

const apiType = "profile"

func New() resource.Resource {
	return &profileResource{}
}

type profileResource struct {
	c *client.Client
}

type model struct {
	ID           types.String         `tfsdk:"id"`
	Email        types.String         `tfsdk:"email"`
	PhoneNumber  types.String         `tfsdk:"phone_number"`
	ExternalID   types.String         `tfsdk:"external_id"`
	FirstName    types.String         `tfsdk:"first_name"`
	LastName     types.String         `tfsdk:"last_name"`
	Organization types.String         `tfsdk:"organization"`
	Locale       types.String         `tfsdk:"locale"`
	Title        types.String         `tfsdk:"title"`
	Image        types.String         `tfsdk:"image"`
	Properties   jsontypes.Normalized `tfsdk:"properties"`
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
	Email        *string         `json:"email,omitempty"`
	PhoneNumber  *string         `json:"phone_number,omitempty"`
	ExternalID   *string         `json:"external_id,omitempty"`
	FirstName    *string         `json:"first_name,omitempty"`
	LastName     *string         `json:"last_name,omitempty"`
	Organization *string         `json:"organization,omitempty"`
	Locale       *string         `json:"locale,omitempty"`
	Title        *string         `json:"title,omitempty"`
	Image        *string         `json:"image,omitempty"`
	Properties   json.RawMessage `json:"properties,omitempty"`
	Created      *string         `json:"created,omitempty"`
	Updated      *string         `json:"updated,omitempty"`
}

func (r *profileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_profile"
}

func (r *profileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Klaviyo subscriber profile. Klaviyo's API does not expose Delete; `terraform destroy` removes the resource from state only — use Klaviyo's data-privacy/suppression flows to actually remove the profile.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Klaviyo-assigned identifier.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"email":         optionalString("Email address (must be unique across the account)."),
			"phone_number":  optionalString("Phone number in E.164 format (e.g. `+15005550006`)."),
			"external_id":   optionalString("Identifier from your upstream system (POS, e-commerce, etc.)."),
			"first_name":    optionalString("First name."),
			"last_name":     optionalString("Last name."),
			"organization":  optionalString("Organization or company name."),
			"locale":        optionalString("IETF BCP 47 locale (e.g. `en-US`)."),
			"title":         optionalString("Job title."),
			"image":         optionalString("URL to a profile image."),
			"properties": schema.StringAttribute{
				Description: "Arbitrary custom properties as a JSON object. Whitespace-only diffs ignored.",
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"created": schema.StringAttribute{
				Description: "ISO-8601 timestamp the profile was created (server-assigned).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated": schema.StringAttribute{
				Description: "ISO-8601 timestamp the profile was last updated (server-assigned).",
				Computed:    true,
			},
		},
	}
}

// optionalString builds an Optional StringAttribute with the given
// description. Cuts repetition since the resource has many of them.
func optionalString(desc string) schema.StringAttribute {
	return schema.StringAttribute{Description: desc, Optional: true}
}

func (r *profileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *profileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan model
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	body := envelope{Data: resourceObject{Type: apiType, Attributes: attributesFromPlan(plan)}}
	out, err := r.send(ctx, http.MethodPost, "/api/profiles/", body, http.StatusCreated)
	if err != nil {
		resp.Diagnostics.AddError("Creating Klaviyo profile failed", err.Error())
		return
	}
	mergeIntoModel(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *profileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state model
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	out, err := r.send(ctx, http.MethodGet, "/api/profiles/"+state.ID.ValueString(), nil, http.StatusOK)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading Klaviyo profile failed", err.Error())
		return
	}
	mergeIntoModel(&state, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *profileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
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
	out, err := r.send(ctx, http.MethodPatch, "/api/profiles/"+plan.ID.ValueString(), body, http.StatusOK)
	if err != nil {
		resp.Diagnostics.AddError("Updating Klaviyo profile failed", err.Error())
		return
	}
	mergeIntoModel(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete: Klaviyo does NOT expose a profile-delete endpoint. We
// remove the resource from Terraform state and warn the operator
// that the upstream record is still present.
func (r *profileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state model
	_ = req.State.Get(ctx, &state)
	resp.Diagnostics.AddWarning(
		"Klaviyo profile cannot be deleted via API",
		fmt.Sprintf("The profile %q has been removed from Terraform state, but Klaviyo's API does not delete profiles. Use Klaviyo's data-privacy/suppression flows (https://developers.klaviyo.com/en/reference/data_privacy) to remove the underlying record if needed.", state.ID.ValueString()),
	)
}

func (r *profileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *profileResource) send(ctx context.Context, method, urlPath string, body any, expectedStatus int) (resourceObject, error) {
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

// stringPtrIfKnown returns &v when s carries a known, non-null value,
// otherwise nil — so omitempty in the wire DTO does the right thing.
func stringPtrIfKnown(s types.String) *string {
	if s.IsUnknown() || s.IsNull() {
		return nil
	}
	v := s.ValueString()
	return &v
}

func attributesFromPlan(m model) attributes {
	a := attributes{
		Email:        stringPtrIfKnown(m.Email),
		PhoneNumber:  stringPtrIfKnown(m.PhoneNumber),
		ExternalID:   stringPtrIfKnown(m.ExternalID),
		FirstName:    stringPtrIfKnown(m.FirstName),
		LastName:     stringPtrIfKnown(m.LastName),
		Organization: stringPtrIfKnown(m.Organization),
		Locale:       stringPtrIfKnown(m.Locale),
		Title:        stringPtrIfKnown(m.Title),
		Image:        stringPtrIfKnown(m.Image),
	}
	if !m.Properties.IsUnknown() && !m.Properties.IsNull() && m.Properties.ValueString() != "" {
		a.Properties = json.RawMessage(m.Properties.ValueString())
	}
	return a
}

func setOrNull(target *types.String, src *string) {
	if src != nil {
		*target = types.StringValue(*src)
	} else {
		*target = types.StringNull()
	}
}

func mergeIntoModel(m *model, r resourceObject) {
	m.ID = types.StringValue(r.ID)
	setOrNull(&m.Email, r.Attributes.Email)
	setOrNull(&m.PhoneNumber, r.Attributes.PhoneNumber)
	setOrNull(&m.ExternalID, r.Attributes.ExternalID)
	setOrNull(&m.FirstName, r.Attributes.FirstName)
	setOrNull(&m.LastName, r.Attributes.LastName)
	setOrNull(&m.Organization, r.Attributes.Organization)
	setOrNull(&m.Locale, r.Attributes.Locale)
	setOrNull(&m.Title, r.Attributes.Title)
	setOrNull(&m.Image, r.Attributes.Image)
	if len(r.Attributes.Properties) > 0 {
		m.Properties = jsontypes.NewNormalizedValue(string(r.Attributes.Properties))
	}
	if r.Attributes.Created != nil {
		m.Created = types.StringValue(*r.Attributes.Created)
	}
	if r.Attributes.Updated != nil {
		m.Updated = types.StringValue(*r.Attributes.Updated)
	}
}
