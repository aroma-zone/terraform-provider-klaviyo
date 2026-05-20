// Package datasource_data_source implements the klaviyo_data_source
// lookup data source.
package datasource_data_source

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
	Title       types.String `tfsdk:"title"`
	Visibility  types.String `tfsdk:"visibility"`
	Description types.String `tfsdk:"description"`
	Namespace   types.String `tfsdk:"namespace"`
}

type envelope struct {
	Data resourceObject `json:"data"`
}

type resourceObject struct {
	ID         string     `json:"id"`
	Attributes attributes `json:"attributes"`
}

type attributes struct {
	Title       string  `json:"title"`
	Visibility  *string `json:"visibility,omitempty"`
	Description *string `json:"description,omitempty"`
	Namespace   *string `json:"namespace,omitempty"`
}

func (d *lookupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_data_source"
}

func (d *lookupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing Klaviyo data source (Custom Objects) by id.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Required: true, Description: "Data source id."},
			"title":       schema.StringAttribute{Computed: true},
			"visibility":  schema.StringAttribute{Computed: true},
			"description": schema.StringAttribute{Computed: true},
			"namespace":   schema.StringAttribute{Computed: true},
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
	httpResp, err := d.c.Do(ctx, http.MethodGet, "/api/data-sources/"+cfg.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Reading Klaviyo data source failed", err.Error())
		return
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		resp.Diagnostics.AddError("Reading Klaviyo data source failed", client.DecodeError(httpResp).Error())
		return
	}
	var env envelope
	if err := json.NewDecoder(httpResp.Body).Decode(&env); err != nil {
		resp.Diagnostics.AddError("Decoding Klaviyo response failed", err.Error())
		return
	}
	a := env.Data.Attributes
	cfg.ID = types.StringValue(env.Data.ID)
	cfg.Title = types.StringValue(a.Title)
	cfg.Visibility = nilSafeString(a.Visibility)
	cfg.Description = nilSafeString(a.Description)
	cfg.Namespace = nilSafeString(a.Namespace)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

func nilSafeString(p *string) types.String {
	if p != nil {
		return types.StringValue(*p)
	}
	return types.StringNull()
}
