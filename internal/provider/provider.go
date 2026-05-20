// Package provider implements the Klaviyo Terraform provider: its schema,
// configuration plumbing, and the registration of every resource and data
// source the provider exposes.
package provider

import (
	"context"
	"os"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/client"
	resourcecoupon "github.com/aroma-zone/terraform-provider-klaviyo/internal/resource_coupon"
	resourcecustomobject "github.com/aroma-zone/terraform-provider-klaviyo/internal/resource_custom_object"
	resourcedatasource "github.com/aroma-zone/terraform-provider-klaviyo/internal/resource_data_source"
	resourceflow "github.com/aroma-zone/terraform-provider-klaviyo/internal/resource_flow"
	resourcelist "github.com/aroma-zone/terraform-provider-klaviyo/internal/resource_list"
	resourcesegment "github.com/aroma-zone/terraform-provider-klaviyo/internal/resource_segment"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// defaultAPIRevision is the Klaviyo API revision this provider was built
// against. The `.pre` suffix unlocks beta endpoints (currently used by
// klaviyo_custom_object) without breaking the stable endpoints used by
// the other resources. It matches spec/version.txt; bump both together
// when regenerating from a newer upstream spec.
const defaultAPIRevision = "2026-04-15.pre"

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

// Configure resolves the provider's config into a *client.Client and
// hands it to every resource and data source via ResourceData /
// DataSourceData. Errors here are surfaced to the user via diagnostics.
func (p *klaviyoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg klaviyoProviderModel
	if diags := req.Config.Get(ctx, &cfg); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	apiKey := firstNonEmpty(cfg.APIKey.ValueString(), os.Getenv("KLAVIYO_API_KEY"))
	revision := firstNonEmpty(cfg.APIRevision.ValueString(), os.Getenv("KLAVIYO_API_REVISION"), defaultAPIRevision)
	baseURL := firstNonEmpty(cfg.BaseURL.ValueString(), client.DefaultBaseURL)

	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Missing Klaviyo API key",
			"Set the `api_key` attribute on the provider block or the `KLAVIYO_API_KEY` environment variable.",
		)
		return
	}

	c := client.New(apiKey, revision,
		client.WithBaseURL(baseURL),
		client.WithUserAgent("terraform-provider-klaviyo/"+p.version),
	)

	// Every resource's Configure receives this same client.
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *klaviyoProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resourcelist.New,
		resourcedatasource.New,
		resourcecoupon.New,
		resourcecustomobject.New,
		resourceflow.New,
		resourcesegment.New,
	}
}

func (p *klaviyoProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

// firstNonEmpty returns the first argument that isn't the empty string,
// or "" if every argument is empty.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
