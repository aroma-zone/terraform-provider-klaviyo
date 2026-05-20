// Package datasource_webhook implements the klaviyo_webhook lookup
// data source. secret_key is intentionally absent (write-only on the
// API). endpoint_url is exposed but flagged in docs as truncated.
package datasource_webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = (*lookupDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*lookupDataSource)(nil)
)

func New() datasource.DataSource { return &lookupDataSource{} }

type lookupDataSource struct {
	c *client.Client
}

type model struct {
	ID                 types.String   `tfsdk:"id"`
	Name               types.String   `tfsdk:"name"`
	Description        types.String   `tfsdk:"description"`
	EndpointURLPrefix  types.String   `tfsdk:"endpoint_url_prefix"`
	Enabled            types.Bool     `tfsdk:"enabled"`
	WebhookTopics      []types.String `tfsdk:"webhook_topics"`
	CreatedAt          types.String   `tfsdk:"created_at"`
	UpdatedAt          types.String   `tfsdk:"updated_at"`
}

type envelope struct {
	Data resourceObject `json:"data"`
}

type resourceObject struct {
	ID            string         `json:"id"`
	Attributes    attributes     `json:"attributes"`
	Relationships *relationships `json:"relationships,omitempty"`
}

type attributes struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	EndpointURL string  `json:"endpoint_url"`
	Enabled     bool    `json:"enabled"`
	CreatedAt   *string `json:"created_at,omitempty"`
	UpdatedAt   *string `json:"updated_at,omitempty"`
}

type relationships struct {
	WebhookTopics *topicList `json:"webhook-topics,omitempty"`
}

type topicList struct {
	Data []topicRef `json:"data"`
}

type topicRef struct {
	ID string `json:"id"`
}

func (d *lookupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook"
}

func (d *lookupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing Klaviyo webhook by id. Note: `secret_key` is not exposed (Klaviyo never returns it on read), and `endpoint_url_prefix` is the truncated value Klaviyo returns for security.",
		Attributes: map[string]schema.Attribute{
			"id":                  schema.StringAttribute{Required: true, Description: "Webhook id."},
			"name":                schema.StringAttribute{Computed: true},
			"description":         schema.StringAttribute{Computed: true},
			"endpoint_url_prefix": schema.StringAttribute{Computed: true, Description: "Truncated endpoint URL (security feature; the full URL is not retrievable from Klaviyo)."},
			"enabled":             schema.BoolAttribute{Computed: true},
			"webhook_topics":      schema.ListAttribute{Computed: true, ElementType: types.StringType},
			"created_at":          schema.StringAttribute{Computed: true},
			"updated_at":          schema.StringAttribute{Computed: true},
		},
	}
}

func (d *lookupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client, got %T.", req.ProviderData))
		return
	}
	d.c = c
}

func (d *lookupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg model
	if diags := req.Config.Get(ctx, &cfg); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	httpResp, err := d.c.Do(ctx, http.MethodGet, "/api/webhooks/"+cfg.ID.ValueString()+"?include=webhook-topics", nil)
	if err != nil {
		resp.Diagnostics.AddError("Reading Klaviyo webhook failed", err.Error())
		return
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		resp.Diagnostics.AddError("Reading Klaviyo webhook failed", client.DecodeError(httpResp).Error())
		return
	}
	var env envelope
	if err := json.NewDecoder(httpResp.Body).Decode(&env); err != nil {
		resp.Diagnostics.AddError("Decoding Klaviyo response failed", err.Error())
		return
	}
	a := env.Data.Attributes
	cfg.ID = types.StringValue(env.Data.ID)
	cfg.Name = types.StringValue(a.Name)
	if a.Description != nil {
		cfg.Description = types.StringValue(*a.Description)
	} else {
		cfg.Description = types.StringNull()
	}
	cfg.EndpointURLPrefix = types.StringValue(a.EndpointURL)
	cfg.Enabled = types.BoolValue(a.Enabled)
	if a.CreatedAt != nil {
		cfg.CreatedAt = types.StringValue(*a.CreatedAt)
	}
	if a.UpdatedAt != nil {
		cfg.UpdatedAt = types.StringValue(*a.UpdatedAt)
	}
	if env.Data.Relationships != nil && env.Data.Relationships.WebhookTopics != nil {
		topics := make([]types.String, 0, len(env.Data.Relationships.WebhookTopics.Data))
		for _, ref := range env.Data.Relationships.WebhookTopics.Data {
			topics = append(topics, types.StringValue(ref.ID))
		}
		cfg.WebhookTopics = topics
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
