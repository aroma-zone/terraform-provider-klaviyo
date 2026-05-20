// Package resource_webhook implements the klaviyo_webhook managed
// resource (Klaviyo outbound webhooks subscribed to event topics).
//
// Two API quirks we honor in the implementation:
//   - secret_key is write-only: Klaviyo accepts it on POST/PATCH but
//     never returns it on GET. We mark it Sensitive in the schema and
//     keep whatever's already in state on read.
//   - endpoint_url is truncated on read (security feature). Same
//     treatment — don't let read overwrite the config-supplied value.
package resource_webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*webhookResource)(nil)
	_ resource.ResourceWithConfigure   = (*webhookResource)(nil)
	_ resource.ResourceWithImportState = (*webhookResource)(nil)
)

const (
	apiType   = "webhook"
	topicType = "webhook-topic"
)

func New() resource.Resource { return &webhookResource{} }

type webhookResource struct {
	c *client.Client
}

type model struct {
	ID            types.String   `tfsdk:"id"`
	Name          types.String   `tfsdk:"name"`
	Description   types.String   `tfsdk:"description"`
	EndpointURL   types.String   `tfsdk:"endpoint_url"`
	SecretKey     types.String   `tfsdk:"secret_key"`
	Enabled       types.Bool     `tfsdk:"enabled"`
	WebhookTopics []types.String `tfsdk:"webhook_topics"`
	CreatedAt     types.String   `tfsdk:"created_at"`
	UpdatedAt     types.String   `tfsdk:"updated_at"`
}

// JSON:API DTOs. Note that the Webhook envelope requires a
// `relationships` block — unique among the resources we model.

type writeEnvelope struct {
	Data writeResourceObject `json:"data"`
}

type writeResourceObject struct {
	Type          string                  `json:"type"`
	ID            string                  `json:"id,omitempty"`
	Attributes    attributes              `json:"attributes"`
	Relationships *writeRelationships     `json:"relationships,omitempty"`
}

type readEnvelope struct {
	Data readResourceObject `json:"data"`
}

type readResourceObject struct {
	Type          string             `json:"type"`
	ID            string             `json:"id"`
	Attributes    attributes         `json:"attributes"`
	Relationships *readRelationships `json:"relationships,omitempty"`
}

type attributes struct {
	Name        string  `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	EndpointURL string  `json:"endpoint_url,omitempty"`
	SecretKey   string  `json:"secret_key,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
	CreatedAt   *string `json:"created_at,omitempty"`
	UpdatedAt   *string `json:"updated_at,omitempty"`
}

type writeRelationships struct {
	WebhookTopics writeTopicList `json:"webhook-topics"`
}

type writeTopicList struct {
	Data []topicRef `json:"data"`
}

type readRelationships struct {
	WebhookTopics *readTopicList `json:"webhook-topics,omitempty"`
}

type readTopicList struct {
	Data []topicRef `json:"data"`
}

type topicRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

func (r *webhookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook"
}

func (r *webhookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Klaviyo outbound webhook. Subscribes to event topics (e.g. `event:klaviyo.sent_sms`) and POSTs payloads to your endpoint, signed with `secret_key`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Klaviyo identifier (ULID).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name":        schema.StringAttribute{Required: true, Description: "Webhook name."},
			"description": schema.StringAttribute{Optional: true, Description: "Webhook description."},
			"endpoint_url": schema.StringAttribute{
				Required:    true,
				Description: "HTTPS URL Klaviyo POSTs to. Klaviyo truncates this in API read responses for security; Terraform keeps the full value from configuration.",
			},
			"secret_key": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "Shared secret Klaviyo uses to sign webhook requests. Write-only — Klaviyo never returns it on read.",
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether the webhook is active. Defaults to true.",
			},
			"webhook_topics": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "Event topics this webhook subscribes to (e.g. `event:klaviyo.sent_sms`).",
			},
			"created_at": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *webhookResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client, got %T.", req.ProviderData))
		return
	}
	r.c = c
}

func (r *webhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan model
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	body := writeEnvelope{Data: writeResourceObject{
		Type:          apiType,
		Attributes:    attributesFromPlan(plan, true),
		Relationships: relationshipsFromPlan(plan),
	}}
	out, err := r.sendWrite(ctx, http.MethodPost, "/api/webhooks/", body, http.StatusCreated)
	if err != nil {
		resp.Diagnostics.AddError("Creating Klaviyo webhook failed", err.Error())
		return
	}
	mergeIntoModel(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *webhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state model
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	out, err := r.sendRead(ctx, http.MethodGet, "/api/webhooks/"+state.ID.ValueString()+"?include=webhook-topics", nil, http.StatusOK)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading Klaviyo webhook failed", err.Error())
		return
	}
	mergeIntoModel(&state, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *webhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan model
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	body := writeEnvelope{Data: writeResourceObject{
		Type:          apiType,
		ID:            plan.ID.ValueString(),
		Attributes:    attributesFromPlan(plan, false),
		Relationships: relationshipsFromPlan(plan),
	}}
	out, err := r.sendWrite(ctx, http.MethodPatch, "/api/webhooks/"+plan.ID.ValueString(), body, http.StatusOK)
	if err != nil {
		resp.Diagnostics.AddError("Updating Klaviyo webhook failed", err.Error())
		return
	}
	mergeIntoModel(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *webhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state model
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	httpResp, err := r.c.Do(ctx, http.MethodDelete, "/api/webhooks/"+state.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Deleting Klaviyo webhook failed", err.Error())
		return
	}
	defer httpResp.Body.Close()
	switch httpResp.StatusCode {
	case http.StatusNoContent, http.StatusOK, http.StatusNotFound:
		return
	default:
		resp.Diagnostics.AddError("Deleting Klaviyo webhook failed", client.DecodeError(httpResp).Error())
	}
}

func (r *webhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// sendWrite/sendRead split the create vs read envelope decoding —
// they differ in the relationships shape.

func (r *webhookResource) sendWrite(ctx context.Context, method, urlPath string, body any, expectedStatus int) (readResourceObject, error) {
	httpResp, err := r.c.Do(ctx, method, urlPath, body)
	if err != nil {
		return readResourceObject{}, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != expectedStatus {
		return readResourceObject{}, client.DecodeError(httpResp)
	}
	var env readEnvelope
	if err := json.NewDecoder(httpResp.Body).Decode(&env); err != nil {
		return readResourceObject{}, fmt.Errorf("decoding Klaviyo response: %w", err)
	}
	return env.Data, nil
}

func (r *webhookResource) sendRead(ctx context.Context, method, urlPath string, body any, expectedStatus int) (readResourceObject, error) {
	return r.sendWrite(ctx, method, urlPath, body, expectedStatus)
}

func attributesFromPlan(m model, includeWriteOnly bool) attributes {
	a := attributes{Name: m.Name.ValueString()}
	if knownString(m.Description) {
		v := m.Description.ValueString()
		a.Description = &v
	}
	if !m.Enabled.IsUnknown() && !m.Enabled.IsNull() {
		v := m.Enabled.ValueBool()
		a.Enabled = &v
	}
	// endpoint_url and secret_key are sent on every write. On update,
	// the API treats them as no-ops when unchanged, but we always send
	// the user's config-supplied values for correctness — they're
	// write-only and the server can't tell us if they drifted.
	if includeWriteOnly || knownString(m.EndpointURL) {
		a.EndpointURL = m.EndpointURL.ValueString()
	}
	if includeWriteOnly || knownString(m.SecretKey) {
		a.SecretKey = m.SecretKey.ValueString()
	}
	return a
}

func relationshipsFromPlan(m model) *writeRelationships {
	if len(m.WebhookTopics) == 0 {
		// Required on create, optional on update — but easier to always
		// send the current list. If the user didn't specify any, send
		// an empty array (Klaviyo will reject on create, which is the
		// right behavior).
		return &writeRelationships{WebhookTopics: writeTopicList{Data: []topicRef{}}}
	}
	refs := make([]topicRef, 0, len(m.WebhookTopics))
	for _, t := range m.WebhookTopics {
		if !t.IsUnknown() && !t.IsNull() {
			refs = append(refs, topicRef{Type: topicType, ID: t.ValueString()})
		}
	}
	return &writeRelationships{WebhookTopics: writeTopicList{Data: refs}}
}

// mergeIntoModel overwrites server-owned fields ONLY. endpoint_url and
// secret_key are write-only from Terraform's perspective (Klaviyo
// returns endpoint_url truncated, never returns secret_key) — we
// preserve whatever the plan/state had.
func mergeIntoModel(m *model, r readResourceObject) {
	m.ID = types.StringValue(r.ID)
	if r.Attributes.Name != "" {
		m.Name = types.StringValue(r.Attributes.Name)
	}
	if r.Attributes.Description != nil {
		m.Description = types.StringValue(*r.Attributes.Description)
	} else {
		m.Description = types.StringNull()
	}
	if r.Attributes.Enabled != nil {
		m.Enabled = types.BoolValue(*r.Attributes.Enabled)
	}
	if r.Attributes.CreatedAt != nil {
		m.CreatedAt = types.StringValue(*r.Attributes.CreatedAt)
	}
	if r.Attributes.UpdatedAt != nil {
		m.UpdatedAt = types.StringValue(*r.Attributes.UpdatedAt)
	}
	if r.Relationships != nil && r.Relationships.WebhookTopics != nil {
		topics := make([]types.String, 0, len(r.Relationships.WebhookTopics.Data))
		for _, ref := range r.Relationships.WebhookTopics.Data {
			topics = append(topics, types.StringValue(ref.ID))
		}
		m.WebhookTopics = topics
	}
}

func knownString(s types.String) bool { return !s.IsUnknown() && !s.IsNull() }
