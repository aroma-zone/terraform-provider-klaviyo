// Package provider implements the Klaviyo Terraform provider: its schema,
// configuration plumbing, and the registration of every resource and data
// source the provider exposes.
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// klaviyoProvider implements provider.Provider.
type klaviyoProvider struct {
	version string
}

// klaviyoProviderModel mirrors the provider block users write in HCL.
type klaviyoProviderModel struct {
	APIKey      types.String `tfsdk:"api_key"`
	APIRevision types.String `tfsdk:"api_revision"`
	BaseURL     types.String `tfsdk:"base_url"`
}

// New returns the provider constructor expected by providerserver.Serve.
// The version string is set at link time from main.version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &klaviyoProvider{version: version}
	}
}

func (p *klaviyoProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "klaviyo"
	resp.Version = p.version
}

func (p *klaviyoProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provider for managing Klaviyo resources via the Klaviyo REST API.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Description: "Klaviyo private API key. Falls back to the `KLAVIYO_API_KEY` environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"api_revision": schema.StringAttribute{
				Description: "Klaviyo API revision (e.g. `2026-04-15`). Falls back to `KLAVIYO_API_REVISION`, then to the revision this provider was built against.",
				Optional:    true,
			},
			"base_url": schema.StringAttribute{
				Description: "Override the Klaviyo API base URL. Defaults to `https://a.klaviyo.com`.",
				Optional:    true,
			},
		},
	}
}

func (p *klaviyoProvider) Configure(_ context.Context, _ provider.ConfigureRequest, _ *provider.ConfigureResponse) {
	// Wired up in the next checkpoint, alongside the HTTP client.
}

func (p *klaviyoProvider) Resources(_ context.Context) []func() resource.Resource {
	return nil
}

func (p *klaviyoProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}
