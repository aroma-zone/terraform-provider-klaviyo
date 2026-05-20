// Package datasource_custom_object implements the
// klaviyo_custom_object lookup data source (beta object schemas).
package datasource_custom_object

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

type envelope struct {
	Data resourceObject `json:"data"`
}

type resourceObject struct {
	ID         string     `json:"id"`
	Attributes attributes `json:"attributes"`
}

type attributes struct {
	Title       string        `json:"title"`
	Description *string       `json:"description,omitempty"`
	Status      *string       `json:"status,omitempty"`
	Required    *[]string     `json:"required,omitempty"`
	Properties  []apiProperty `json:"properties"`
	PublishedAt *string       `json:"published_at,omitempty"`
	Visibility  *string       `json:"visibility,omitempty"`
}

type apiProperty struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Description *string `json:"description,omitempty"`
}

func (d *lookupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_object"
}

func (d *lookupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing Klaviyo Custom Object schema (beta) by id.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Required: true, Description: "Object schema id."},
			"title":        schema.StringAttribute{Computed: true},
			"description":  schema.StringAttribute{Computed: true},
			"status":       schema.StringAttribute{Computed: true},
			"required":     schema.ListAttribute{Computed: true, ElementType: types.StringType},
			"published_at": schema.StringAttribute{Computed: true},
			"visibility":   schema.StringAttribute{Computed: true},
			"properties": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.Int64Attribute{Computed: true},
						"name":        schema.StringAttribute{Computed: true},
						"type":        schema.StringAttribute{Computed: true},
						"description": schema.StringAttribute{Computed: true},
					},
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
	httpResp, err := d.c.Do(ctx, http.MethodGet, "/api/object-schemas/"+cfg.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Reading Klaviyo custom object failed", err.Error())
		return
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		resp.Diagnostics.AddError("Reading Klaviyo custom object failed", client.DecodeError(httpResp).Error())
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
	cfg.Description = nilSafe(a.Description)
	cfg.Status = nilSafe(a.Status)
	cfg.PublishedAt = nilSafe(a.PublishedAt)
	cfg.Visibility = nilSafe(a.Visibility)
	if a.Required != nil {
		cfg.Required = make([]types.String, 0, len(*a.Required))
		for _, s := range *a.Required {
			cfg.Required = append(cfg.Required, types.StringValue(s))
		}
	}
	cfg.Properties = make([]propertyBlock, 0, len(a.Properties))
	for _, p := range a.Properties {
		pb := propertyBlock{
			ID:          types.Int64Value(p.ID),
			Name:        types.StringValue(p.Name),
			Type:        types.StringValue(p.Type),
			Description: nilSafe(p.Description),
		}
		cfg.Properties = append(cfg.Properties, pb)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

func nilSafe(p *string) types.String {
	if p != nil {
		return types.StringValue(*p)
	}
	return types.StringNull()
}
