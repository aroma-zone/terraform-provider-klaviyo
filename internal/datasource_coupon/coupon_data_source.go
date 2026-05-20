// Package datasource_coupon implements the klaviyo_coupon lookup
// data source.
package datasource_coupon

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
	ID                   types.String        `tfsdk:"id"`
	ExternalID           types.String        `tfsdk:"external_id"`
	Description          types.String        `tfsdk:"description"`
	MonitorConfiguration *monitorConfigBlock `tfsdk:"monitor_configuration"`
}

type monitorConfigBlock struct {
	LowBalanceThreshold types.Int64 `tfsdk:"low_balance_threshold"`
}

type envelope struct {
	Data resourceObject `json:"data"`
}

type resourceObject struct {
	ID         string     `json:"id"`
	Attributes attributes `json:"attributes"`
}

type attributes struct {
	ExternalID           string                `json:"external_id"`
	Description          *string               `json:"description,omitempty"`
	MonitorConfiguration *monitorConfiguration `json:"monitor_configuration,omitempty"`
}

type monitorConfiguration struct {
	LowBalanceThreshold *int64 `json:"low_balance_threshold,omitempty"`
}

func (d *lookupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_coupon"
}

func (d *lookupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing Klaviyo coupon by id.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Required: true, Description: "Coupon id (equal to external_id)."},
			"external_id": schema.StringAttribute{Computed: true},
			"description": schema.StringAttribute{Computed: true},
			"monitor_configuration": schema.SingleNestedAttribute{
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"low_balance_threshold": schema.Int64Attribute{Computed: true},
				},
			},
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
	httpResp, err := d.c.Do(ctx, http.MethodGet, "/api/coupons/"+cfg.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Reading Klaviyo coupon failed", err.Error())
		return
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		resp.Diagnostics.AddError("Reading Klaviyo coupon failed", client.DecodeError(httpResp).Error())
		return
	}
	var env envelope
	if err := json.NewDecoder(httpResp.Body).Decode(&env); err != nil {
		resp.Diagnostics.AddError("Decoding Klaviyo response failed", err.Error())
		return
	}
	a := env.Data.Attributes
	cfg.ID = types.StringValue(env.Data.ID)
	cfg.ExternalID = types.StringValue(a.ExternalID)
	if a.Description != nil {
		cfg.Description = types.StringValue(*a.Description)
	} else {
		cfg.Description = types.StringNull()
	}
	if a.MonitorConfiguration != nil {
		block := &monitorConfigBlock{}
		if a.MonitorConfiguration.LowBalanceThreshold != nil {
			block.LowBalanceThreshold = types.Int64Value(*a.MonitorConfiguration.LowBalanceThreshold)
		} else {
			block.LowBalanceThreshold = types.Int64Null()
		}
		cfg.MonitorConfiguration = block
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
