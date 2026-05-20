// Package datasource_list implements the klaviyo_list lookup
// data source: read-only access to an existing Klaviyo list by id.
package datasource_list

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
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	OptInProcess types.String `tfsdk:"opt_in_process"`
	Created      types.String `tfsdk:"created"`
	Updated      types.String `tfsdk:"updated"`
}

type envelope struct {
	Data resourceObject `json:"data"`
}

type resourceObject struct {
	ID         string     `json:"id"`
	Attributes attributes `json:"attributes"`
}

type attributes struct {
	Name         string  `json:"name"`
	OptInProcess *string `json:"opt_in_process,omitempty"`
	Created      string  `json:"created,omitempty"`
	Updated      string  `json:"updated,omitempty"`
}

func (d *lookupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_list"
}

func (d *lookupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing Klaviyo list by id.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Required: true, Description: "Klaviyo list id."},
			"name":           schema.StringAttribute{Computed: true, Description: "List name."},
			"opt_in_process": schema.StringAttribute{Computed: true, Description: "`double_opt_in` or `single_opt_in`."},
			"created":        schema.StringAttribute{Computed: true},
			"updated":        schema.StringAttribute{Computed: true},
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
	httpResp, err := d.c.Do(ctx, http.MethodGet, "/api/lists/"+cfg.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Reading Klaviyo list failed", err.Error())
		return
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		resp.Diagnostics.AddError("Reading Klaviyo list failed", client.DecodeError(httpResp).Error())
		return
	}
	var env envelope
	if err := json.NewDecoder(httpResp.Body).Decode(&env); err != nil {
		resp.Diagnostics.AddError("Decoding Klaviyo response failed", err.Error())
		return
	}
	cfg.ID = types.StringValue(env.Data.ID)
	cfg.Name = types.StringValue(env.Data.Attributes.Name)
	if env.Data.Attributes.OptInProcess != nil {
		cfg.OptInProcess = types.StringValue(*env.Data.Attributes.OptInProcess)
	} else {
		cfg.OptInProcess = types.StringNull()
	}
	cfg.Created = types.StringValue(env.Data.Attributes.Created)
	cfg.Updated = types.StringValue(env.Data.Attributes.Updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
