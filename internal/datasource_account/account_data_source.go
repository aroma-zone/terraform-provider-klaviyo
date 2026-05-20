// Package datasource_account implements the klaviyo_account
// data source. Klaviyo's private API key is scoped to a single
// account, so the data source defaults to "the account this key
// belongs to"; passing `id` explicitly is supported for the rare
// case of an API key with multi-account access.
package datasource_account

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
	_ datasource.DataSource              = (*accountDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*accountDataSource)(nil)
)

// New is the constructor used by provider.DataSources().
func New() datasource.DataSource {
	return &accountDataSource{}
}

type accountDataSource struct {
	c *client.Client
}

type model struct {
	ID                 types.String         `tfsdk:"id"`
	TestAccount        types.Bool           `tfsdk:"test_account"`
	Industry           types.String         `tfsdk:"industry"`
	Timezone           types.String         `tfsdk:"timezone"`
	PreferredCurrency  types.String         `tfsdk:"preferred_currency"`
	PublicAPIKey       types.String         `tfsdk:"public_api_key"`
	Locale             types.String         `tfsdk:"locale"`
	ContactInformation *contactInformation  `tfsdk:"contact_information"`
}

type contactInformation struct {
	DefaultSenderName  types.String `tfsdk:"default_sender_name"`
	DefaultSenderEmail types.String `tfsdk:"default_sender_email"`
	WebsiteURL         types.String `tfsdk:"website_url"`
	OrganizationName   types.String `tfsdk:"organization_name"`
}

// API DTOs.

type listEnvelope struct {
	Data []resourceObject `json:"data"`
}

type singleEnvelope struct {
	Data resourceObject `json:"data"`
}

type resourceObject struct {
	Type       string     `json:"type"`
	ID         string     `json:"id"`
	Attributes attributes `json:"attributes"`
}

type attributes struct {
	TestAccount        bool                  `json:"test_account"`
	Industry           *string               `json:"industry,omitempty"`
	Timezone           string                `json:"timezone"`
	PreferredCurrency  string                `json:"preferred_currency"`
	PublicAPIKey       string                `json:"public_api_key"`
	Locale             string                `json:"locale"`
	ContactInformation *contactInformationDTO `json:"contact_information,omitempty"`
}

type contactInformationDTO struct {
	DefaultSenderName  string  `json:"default_sender_name"`
	DefaultSenderEmail string  `json:"default_sender_email"`
	WebsiteURL         *string `json:"website_url,omitempty"`
	OrganizationName   string  `json:"organization_name"`
}

func (d *accountDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account"
}

func (d *accountDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Read-only metadata about a Klaviyo account. Defaults to the account associated with the provider's API key; specify `id` to fetch a different account when the key has multi-account access.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Klaviyo account ID. If unset, defaults to the single account the API key is scoped to.",
				Optional:    true,
				Computed:    true,
			},
			"test_account": schema.BoolAttribute{
				Description: "True if this is a Klaviyo test account.",
				Computed:    true,
			},
			"industry":           schema.StringAttribute{Computed: true, Description: "Industry classification."},
			"timezone":           schema.StringAttribute{Computed: true, Description: "IANA timezone."},
			"preferred_currency": schema.StringAttribute{Computed: true, Description: "Preferred currency code (e.g. `USD`)."},
			"public_api_key":     schema.StringAttribute{Computed: true, Description: "Public API key (safe to ship to clients)."},
			"locale":             schema.StringAttribute{Computed: true, Description: "Account locale (e.g. `en-US`)."},
			"contact_information": schema.SingleNestedAttribute{
				Description: "CAN-SPAM contact information used in email footers.",
				Computed:    true,
				Attributes: map[string]schema.Attribute{
					"default_sender_name":  schema.StringAttribute{Computed: true},
					"default_sender_email": schema.StringAttribute{Computed: true},
					"website_url":          schema.StringAttribute{Computed: true},
					"organization_name":    schema.StringAttribute{Computed: true},
				},
			},
		},
	}
}

func (d *accountDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
	d.c = c
}

func (d *accountDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg model
	if diags := req.Config.Get(ctx, &cfg); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	// If no id was specified, list and pick the first.
	var account resourceObject
	if cfg.ID.IsNull() || cfg.ID.IsUnknown() {
		acc, err := d.fetchFirst(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Reading Klaviyo account failed", err.Error())
			return
		}
		account = acc
	} else {
		acc, err := d.fetchByID(ctx, cfg.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading Klaviyo account failed", err.Error())
			return
		}
		account = acc
	}

	mergeIntoModel(&cfg, account)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

func (d *accountDataSource) fetchFirst(ctx context.Context) (resourceObject, error) {
	httpResp, err := d.c.Do(ctx, http.MethodGet, "/api/accounts/", nil)
	if err != nil {
		return resourceObject{}, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		return resourceObject{}, client.DecodeError(httpResp)
	}
	var env listEnvelope
	if err := json.NewDecoder(httpResp.Body).Decode(&env); err != nil {
		return resourceObject{}, fmt.Errorf("decoding /api/accounts response: %w", err)
	}
	if len(env.Data) == 0 {
		return resourceObject{}, fmt.Errorf("Klaviyo returned no accounts for this API key")
	}
	return env.Data[0], nil
}

func (d *accountDataSource) fetchByID(ctx context.Context, id string) (resourceObject, error) {
	httpResp, err := d.c.Do(ctx, http.MethodGet, "/api/accounts/"+id, nil)
	if err != nil {
		return resourceObject{}, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		return resourceObject{}, client.DecodeError(httpResp)
	}
	var env singleEnvelope
	if err := json.NewDecoder(httpResp.Body).Decode(&env); err != nil {
		return resourceObject{}, fmt.Errorf("decoding /api/accounts/%s response: %w", id, err)
	}
	return env.Data, nil
}

func mergeIntoModel(m *model, r resourceObject) {
	m.ID = types.StringValue(r.ID)
	m.TestAccount = types.BoolValue(r.Attributes.TestAccount)
	if r.Attributes.Industry != nil {
		m.Industry = types.StringValue(*r.Attributes.Industry)
	} else {
		m.Industry = types.StringNull()
	}
	m.Timezone = types.StringValue(r.Attributes.Timezone)
	m.PreferredCurrency = types.StringValue(r.Attributes.PreferredCurrency)
	m.PublicAPIKey = types.StringValue(r.Attributes.PublicAPIKey)
	m.Locale = types.StringValue(r.Attributes.Locale)
	if r.Attributes.ContactInformation != nil {
		ci := &contactInformation{
			DefaultSenderName:  types.StringValue(r.Attributes.ContactInformation.DefaultSenderName),
			DefaultSenderEmail: types.StringValue(r.Attributes.ContactInformation.DefaultSenderEmail),
			OrganizationName:   types.StringValue(r.Attributes.ContactInformation.OrganizationName),
		}
		if r.Attributes.ContactInformation.WebsiteURL != nil {
			ci.WebsiteURL = types.StringValue(*r.Attributes.ContactInformation.WebsiteURL)
		} else {
			ci.WebsiteURL = types.StringNull()
		}
		m.ContactInformation = ci
	}
}
