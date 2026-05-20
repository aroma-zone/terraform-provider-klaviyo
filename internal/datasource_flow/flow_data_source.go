// Package datasource_flow implements the klaviyo_flow lookup
// data source. Note: Klaviyo's API does not return the FlowDefinition
// on read, so this data source exposes only the surface metadata.
package datasource_flow

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
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Status      types.String `tfsdk:"status"`
	Archived    types.Bool   `tfsdk:"archived"`
	TriggerType types.String `tfsdk:"trigger_type"`
	Created     types.String `tfsdk:"created"`
	Updated     types.String `tfsdk:"updated"`
}

type envelope struct {
	Data resourceObject `json:"data"`
}

type resourceObject struct {
	ID         string     `json:"id"`
	Attributes attributes `json:"attributes"`
}

type attributes struct {
	Name        string  `json:"name,omitempty"`
	Status      string  `json:"status,omitempty"`
	Archived    *bool   `json:"archived,omitempty"`
	TriggerType *string `json:"trigger_type,omitempty"`
	Created     *string `json:"created,omitempty"`
	Updated     *string `json:"updated,omitempty"`
}

func (d *lookupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_flow"
}

func (d *lookupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing Klaviyo flow by id. Note: Klaviyo's read endpoint does not return the FlowDefinition blob, only the surface metadata.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Required: true, Description: "Flow id."},
			"name":         schema.StringAttribute{Computed: true},
			"status":       schema.StringAttribute{Computed: true},
			"archived":     schema.BoolAttribute{Computed: true},
			"trigger_type": schema.StringAttribute{Computed: true},
			"created":      schema.StringAttribute{Computed: true},
			"updated":      schema.StringAttribute{Computed: true},
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
	httpResp, err := d.c.Do(ctx, http.MethodGet, "/api/flows/"+cfg.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Reading Klaviyo flow failed", err.Error())
		return
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		resp.Diagnostics.AddError("Reading Klaviyo flow failed", client.DecodeError(httpResp).Error())
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
	cfg.Status = types.StringValue(a.Status)
	if a.Archived != nil {
		cfg.Archived = types.BoolValue(*a.Archived)
	} else {
		cfg.Archived = types.BoolNull()
	}
	if a.TriggerType != nil {
		cfg.TriggerType = types.StringValue(*a.TriggerType)
	} else {
		cfg.TriggerType = types.StringNull()
	}
	if a.Created != nil {
		cfg.Created = types.StringValue(*a.Created)
	}
	if a.Updated != nil {
		cfg.Updated = types.StringValue(*a.Updated)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
