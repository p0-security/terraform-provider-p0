// Copyright (c) HashiCorp, Inc. and P0 Security, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/provider/resources"
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
	installgcp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/gcp"
	installssh "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/ssh"
)

// Ensure P0Provider satisfies various provider interfaces.
var _ provider.Provider = &P0Provider{}
var _ provider.ProviderWithFunctions = &P0Provider{}

// P0Provider defines the provider implementation.
type P0Provider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// P0ProviderModel describes the provider data model.
type P0ProviderModel struct {
	Host types.String `tfsdk:"host"`
	Org  types.String `tfsdk:"org"`
}

func (p *P0Provider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "p0"
	resp.Version = p.version
}

func (p *P0Provider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Configures a P0 organization. Requires a P0 account. Go to https://p0.app to create an account.

You must also configure a P0 API token (on your P0 app "/settings" page). Then run Terraform with your API token in
the P0_API_TOKEN environment variable.`,
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				MarkdownDescription: "Your P0 application API host (defaults to `https://api.p0.app`)",
				Optional:            true,
			},
			"org": schema.StringAttribute{
				MarkdownDescription: "Your P0 organization identifier.",
				Required:            true,
			},
		},
	}
}

func (p *P0Provider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var model P0ProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)

	if model.Org.IsUnknown() {
		resp.Diagnostics.AddError(
			"P0 organization identifier is required",
			"This is the identifier you use when logging in to https://p0.app.",
		)
	}

	api_token := os.Getenv("P0_API_TOKEN")
	if api_token == "" {
		resp.Diagnostics.AddError(
			"No P0_API_TOKEN environment variable",
			fmt.Sprintf(
				"A P0 API token is required to use the P0 Terraform provider. To create a token, navigate to https://p0.app/o/%s/settings. Pass your token by setting it in the P0_API_TOKEN environment variable.",
				model.Org.ValueString(),
			),
		)
	}

	p0_host := model.Host.ValueString()
	if p0_host == "" {
		p0_host = "https://api.p0.app"
	}

	if resp.Diagnostics.HasError() {
		return
	}

	data := internal.P0ProviderData{
		Authentication: fmt.Sprintf("Bearer %s", api_token),
		Client:         http.DefaultClient,
		BaseUrl:        fmt.Sprintf("%s/o/%s", p0_host, model.Org.ValueString()),
	}
	resp.DataSourceData = data
	resp.ResourceData = data
}

func (p *P0Provider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewRoutingRules,
		installaws.NewAwsIamWrite,
		installaws.NewIamWriteStagedAws,
		installgcp.NewGcp,
		installgcp.NewGcpAccessLogs,
		installgcp.NewGcpIamAssessment,
		installgcp.NewGcpIamAssessmentStaged,
		installgcp.NewGcpIamWrite,
		installgcp.NewGcpIamWriteStaged,
		installgcp.NewGcpOrgAccessLogs,
		installgcp.NewGcpSharingRestriction,
		installssh.NewSshAwsIamWrite,
		installssh.NewSshGcpIamWrite,
	}
}

func (p *P0Provider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func (p *P0Provider) Functions(ctx context.Context) []func() function.Function {
	return []func() function.Function{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &P0Provider{
			version: version,
		}
	}
}
