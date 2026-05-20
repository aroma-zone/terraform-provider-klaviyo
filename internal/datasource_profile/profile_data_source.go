// Package datasource_profile implements the klaviyo_profile lookup
// data source.
package datasource_profile

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
	ID         string     `json:"id"`
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

func (d *lookupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_profile"
}

func (d *lookupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	c := func() schema.StringAttribute { return schema.StringAttribute{Computed: true} }
	resp.Schema = schema.Schema{
		Description: "Look up an existing Klaviyo profile by id.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Required: true, Description: "Profile id."},
			"email":        c(),
			"phone_number": c(),
			"external_id":  c(),
			"first_name":   c(),
			"last_name":    c(),
			"organization": c(),
			"locale":       c(),
			"title":        c(),
			"image":        c(),
			"properties":   schema.StringAttribute{Computed: true, CustomType: jsontypes.NormalizedType{}},
			"created":      c(),
			"updated":      c(),
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
	httpResp, err := d.c.Do(ctx, http.MethodGet, "/api/profiles/"+cfg.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Reading Klaviyo profile failed", err.Error())
		return
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		resp.Diagnostics.AddError("Reading Klaviyo profile failed", client.DecodeError(httpResp).Error())
		return
	}
	var env envelope
	if err := json.NewDecoder(httpResp.Body).Decode(&env); err != nil {
		resp.Diagnostics.AddError("Decoding Klaviyo response failed", err.Error())
		return
	}
	a := env.Data.Attributes
	cfg.ID = types.StringValue(env.Data.ID)
	cfg.Email = nilSafe(a.Email)
	cfg.PhoneNumber = nilSafe(a.PhoneNumber)
	cfg.ExternalID = nilSafe(a.ExternalID)
	cfg.FirstName = nilSafe(a.FirstName)
	cfg.LastName = nilSafe(a.LastName)
	cfg.Organization = nilSafe(a.Organization)
	cfg.Locale = nilSafe(a.Locale)
	cfg.Title = nilSafe(a.Title)
	cfg.Image = nilSafe(a.Image)
	if len(a.Properties) > 0 {
		cfg.Properties = jsontypes.NewNormalizedValue(string(a.Properties))
	}
	if a.Created != nil {
		cfg.Created = types.StringValue(*a.Created)
	}
	if a.Updated != nil {
		cfg.Updated = types.StringValue(*a.Updated)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

func nilSafe(p *string) types.String {
	if p != nil {
		return types.StringValue(*p)
	}
	return types.StringNull()
}
