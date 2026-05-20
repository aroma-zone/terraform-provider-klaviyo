// Package resource_custom_object implements the klaviyo_custom_object
// managed resource, backed by Klaviyo's beta endpoint
// /api/object-schemas (revision 2026-04-15.pre). It maps a Klaviyo
// "object schema" — the structural definition of a custom-objects
// type — onto idiomatic Terraform.
//
// Caveats baked into v0.1.0:
//   - Klaviyo's API exposes Create/Read/Update on object schemas but
//     NOT Delete. terraform destroy removes the resource from state
//     and emits a warning; the upstream schema must be deleted out of
//     band via the Klaviyo UI.
//   - The source-mapping and relationship sub-objects from the spec
//     are not surfaced yet (single-resource scope). Add when needed.
package resource_custom_object

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
	_ resource.Resource                = (*customObjectResource)(nil)
	_ resource.ResourceWithConfigure   = (*customObjectResource)(nil)
	_ resource.ResourceWithImportState = (*customObjectResource)(nil)
)

const apiType = "object-schema"

// New returns a constructor for the klaviyo_custom_object resource.
func New() resource.Resource {
	return &customObjectResource{}
}

type customObjectResource struct {
	c *client.Client
}

// model mirrors the HCL block.
type model struct {
	ID          types.String    `tfsdk:"id"`
	Title       types.String    `tfsdk:"title"`
	Description types.String    `tfsdk:"description"`
	Status      types.String    `tfsdk:"status"`
	Required    []types.String  `tfsdk:"required"`
	Properties  []propertyBlock `tfsdk:"properties"`
	PublishedAt types.String    `tfsdk:"published_at"`
	Visibility  types.String    `tfsdk:"visibility"`
}

type propertyBlock struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Type        types.String `tfsdk:"type"`
	Description types.String `tfsdk:"description"`
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
	Title       string             `json:"title,omitempty"`
	Description *string            `json:"description,omitempty"`
	Status      *string            `json:"status,omitempty"`
	Properties  []apiProperty      `json:"properties"`
	Required    *[]string          `json:"required,omitempty"`
	PublishedAt *string            `json:"published_at,omitempty"`
	Visibility  *string            `json:"visibility,omitempty"`
}

type apiProperty struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Description *string `json:"description,omitempty"`
}

func (r *customObjectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_object"
}

func (r *customObjectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Klaviyo Custom Object schema (beta — /api/object-schemas). Defines the structure of a custom-objects type. The API does not currently expose Delete, so `terraform destroy` only removes the resource from Terraform state; the upstream schema must be removed via the Klaviyo UI.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Klaviyo-assigned ULID for the object schema.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"title": schema.StringAttribute{
				Description: "Title shown in the Klaviyo UI.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "Human-readable description.",
				Optional:    true,
			},
			"status": schema.StringAttribute{
				Description: "`DRAFT` (default) or `ACTIVE`. Note: the Klaviyo backend may report transient values (`PUBLISHING`, `UNDEFINED`) during state changes; those are surfaced read-only.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("DRAFT"),
				Validators: []validator.String{
					// Accept the lifecycle states the API may report. We
					// only ever WRITE DRAFT or ACTIVE.
					stringvalidator.OneOf("ACTIVE", "DRAFT", "PUBLISHING", "UNDEFINED"),
				},
			},
			"required": schema.ListAttribute{
				Description: "Names of properties that are required on records of this object schema.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"properties": schema.ListNestedAttribute{
				Description: "Property definitions. At least one is required.",
				Required:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Description: "Stable positive-integer identifier for the property within the schema.",
							Required:    true,
						},
						"name": schema.StringAttribute{
							Description: "Property name.",
							Required:    true,
						},
						"type": schema.StringAttribute{
							Description: "Property type. One of `BOOLEAN`, `FLOAT`, `INT`, `STRING`, `TIMESTAMP`.",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.OneOf("BOOLEAN", "FLOAT", "INT", "STRING", "TIMESTAMP"),
							},
						},
						"description": schema.StringAttribute{
							Description: "Human-readable description for this property.",
							Optional:    true,
						},
					},
				},
			},
			"published_at": schema.StringAttribute{
				Description: "ISO-8601 timestamp when the schema was last activated. Server-assigned.",
				Computed:    true,
			},
			"visibility": schema.StringAttribute{
				Description: "Server-side visibility marker (`PRIVATE` or `SHARED`).",
				Computed:    true,
			},
		},
	}
}

func (r *customObjectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *customObjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan model
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	body := envelope{Data: resourceObject{
		Type:       apiType,
		Attributes: attributesFromPlan(plan),
	}}
	out, err := r.send(ctx, http.MethodPost, "/api/object-schemas/", body, http.StatusCreated)
	if err != nil {
		resp.Diagnostics.AddError("Creating Klaviyo custom object failed", err.Error())
		return
	}
	mergeIntoModel(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *customObjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state model
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	out, err := r.send(ctx, http.MethodGet, "/api/object-schemas/"+state.ID.ValueString(), nil, http.StatusOK)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading Klaviyo custom object failed", err.Error())
		return
	}
	mergeIntoModel(&state, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *customObjectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
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
	out, err := r.send(ctx, http.MethodPatch, "/api/object-schemas/"+plan.ID.ValueString(), body, http.StatusOK)
	if err != nil {
		resp.Diagnostics.AddError("Updating Klaviyo custom object failed", err.Error())
		return
	}
	mergeIntoModel(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete removes the resource from Terraform state but cannot remove
// the upstream resource because Klaviyo's API does not expose a DELETE
// for object schemas. Emits a warning so the operator notices.
func (r *customObjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state model
	_ = req.State.Get(ctx, &state)
	resp.Diagnostics.AddWarning(
		"Klaviyo custom object cannot be deleted via API",
		fmt.Sprintf("The object schema %q has been removed from Terraform state, but Klaviyo does not expose a DELETE endpoint for /api/object-schemas. Delete it manually from the Klaviyo UI if no longer needed.", state.ID.ValueString()),
	)
}

func (r *customObjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *customObjectResource) send(ctx context.Context, method, urlPath string, body any, expectedStatus int) (resourceObject, error) {
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
		Title:      m.Title.ValueString(),
		Properties: propertiesToAPI(m.Properties),
	}
	if knownString(m.Description) {
		v := m.Description.ValueString()
		a.Description = &v
	}
	if knownString(m.Status) {
		v := m.Status.ValueString()
		a.Status = &v
	}
	if m.Required != nil {
		req := make([]string, 0, len(m.Required))
		for _, s := range m.Required {
			if !s.IsUnknown() && !s.IsNull() {
				req = append(req, s.ValueString())
			}
		}
		a.Required = &req
	}
	return a
}

func propertiesToAPI(props []propertyBlock) []apiProperty {
	out := make([]apiProperty, 0, len(props))
	for _, p := range props {
		ap := apiProperty{
			ID:   p.ID.ValueInt64(),
			Name: p.Name.ValueString(),
			Type: p.Type.ValueString(),
		}
		if knownString(p.Description) {
			d := p.Description.ValueString()
			ap.Description = &d
		}
		out = append(out, ap)
	}
	return out
}

func mergeIntoModel(m *model, r resourceObject) {
	m.ID = types.StringValue(r.ID)
	m.Title = types.StringValue(r.Attributes.Title)
	if r.Attributes.Description != nil {
		m.Description = types.StringValue(*r.Attributes.Description)
	} else {
		m.Description = types.StringNull()
	}
	if r.Attributes.Status != nil {
		m.Status = types.StringValue(*r.Attributes.Status)
	} else {
		m.Status = types.StringNull()
	}
	if r.Attributes.Visibility != nil {
		m.Visibility = types.StringValue(*r.Attributes.Visibility)
	} else {
		m.Visibility = types.StringNull()
	}
	if r.Attributes.PublishedAt != nil {
		m.PublishedAt = types.StringValue(*r.Attributes.PublishedAt)
	} else {
		m.PublishedAt = types.StringNull()
	}
	if r.Attributes.Required != nil {
		m.Required = make([]types.String, 0, len(*r.Attributes.Required))
		for _, s := range *r.Attributes.Required {
			m.Required = append(m.Required, types.StringValue(s))
		}
	} else {
		m.Required = nil
	}
	m.Properties = make([]propertyBlock, 0, len(r.Attributes.Properties))
	for _, p := range r.Attributes.Properties {
		pb := propertyBlock{
			ID:   types.Int64Value(p.ID),
			Name: types.StringValue(p.Name),
			Type: types.StringValue(p.Type),
		}
		if p.Description != nil {
			pb.Description = types.StringValue(*p.Description)
		} else {
			pb.Description = types.StringNull()
		}
		m.Properties = append(m.Properties, pb)
	}
}

func knownString(s types.String) bool { return !s.IsUnknown() && !s.IsNull() }
