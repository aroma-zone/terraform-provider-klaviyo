// Package datasource_segment implements the klaviyo_segment lookup
// data source.
package datasource_segment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
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
	ID         string     `json:"id"`
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

func (d *lookupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_segment"
}

func (d *lookupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing Klaviyo segment by id.",
		Attributes: map[string]schema.Attribute{
			"id":            schema.StringAttribute{Required: true, Description: "Segment id."},
			"name":          schema.StringAttribute{Computed: true},
			"definition":    schema.StringAttribute{Computed: true, CustomType: jsontypes.NormalizedType{}, Description: "SegmentDefinition JSON."},
			"is_starred":    schema.BoolAttribute{Computed: true},
			"is_active":     schema.BoolAttribute{Computed: true},
			"is_processing": schema.BoolAttribute{Computed: true},
			"created":       schema.StringAttribute{Computed: true},
			"updated":       schema.StringAttribute{Computed: true},
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
	httpResp, err := d.c.Do(ctx, http.MethodGet, "/api/segments/"+cfg.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Reading Klaviyo segment failed", err.Error())
		return
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		resp.Diagnostics.AddError("Reading Klaviyo segment failed", client.DecodeError(httpResp).Error())
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
	if len(a.Definition) > 0 {
		cfg.Definition = jsontypes.NewNormalizedValue(string(a.Definition))
	}
	if a.IsStarred != nil {
		cfg.IsStarred = types.BoolValue(*a.IsStarred)
	}
	if a.IsActive != nil {
		cfg.IsActive = types.BoolValue(*a.IsActive)
	}
	if a.IsProcessing != nil {
		cfg.IsProcessing = types.BoolValue(*a.IsProcessing)
	}
	if a.Created != nil {
		cfg.Created = types.StringValue(*a.Created)
	}
	if a.Updated != nil {
		cfg.Updated = types.StringValue(*a.Updated)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
