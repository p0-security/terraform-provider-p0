// Copyright (c) HashiCorp, Inc. and P0 Security, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/p0-security/terraform-provider-p0/internal"
	installdatadog "github.com/p0-security/terraform-provider-p0/internal/provider/event_collectors/install/datadog"
	installsplunk "github.com/p0-security/terraform-provider-p0/internal/provider/event_collectors/install/splunk"
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
	installazure "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/azure"
	installgcp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/gcp"
	installk8s "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/k8s"
	installmysql "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/mysql"
	installokta "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/okta"
	installpostgres "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/postgres"
	installrds "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/rds"
	installssh "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/ssh"
	routingrules "github.com/p0-security/terraform-provider-p0/internal/provider/resources/routing_rules"
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
	Host     types.String `tfsdk:"host"`
	Org      types.String `tfsdk:"org"`
	ApiToken types.String `tfsdk:"api_token"`
}

func (p *P0Provider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "p0"
	resp.Version = p.version
}

func (p *P0Provider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Configures a P0 organization. Requires a P0 account. Go to https://p0.app to create an account.

You must also configure a P0 API token (on your P0 app "/settings" page). Pass it via the ` + "`api_token`" + ` provider
attribute, or by setting the ` + "`P0_API_TOKEN`" + ` environment variable. The ` + "`api_token`" + ` attribute takes
precedence when both are set.`,
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				MarkdownDescription: "Your P0 application API host (defaults to `https://api.p0.app`)",
				Optional:            true,
			},
			"org": schema.StringAttribute{
				MarkdownDescription: "Your P0 organization identifier",
				Required:            true,
			},
			"api_token": schema.StringAttribute{
				MarkdownDescription: "Your P0 API token. If unset, falls back to the `P0_API_TOKEN` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
		},
	}
}

// resolveApiToken returns the P0 API token to authenticate with, consulting the
// following sources in order of precedence: the api_token provider attribute,
// the P0_API_TOKEN environment variable, and the P0 CLI session (whose OIDC
// credential is exchanged for a Firebase ID token).
func resolveApiToken(ctx context.Context, model P0ProviderModel, diags *diag.Diagnostics) string {
	var token string
	if !model.ApiToken.IsNull() && !model.ApiToken.IsUnknown() {
		token = model.ApiToken.ValueString()
	} else if envToken, ok := os.LookupEnv("P0_API_TOKEN"); ok {
		token = envToken
	} else {
		cliToken, err := cliFirebaseToken(ctx)
		if err != nil {
			diags.AddError("Could not authenticate using the P0 CLI session", err.Error())
			return ""
		}
		token = cliToken
	}
	if token == "" {
		diags.AddError(
			"No P0 authentication configured",
			fmt.Sprintf(
				"Authentication is required to use the P0 Terraform provider. Either login via the P0 CLI or provide an API token via the `api_token` provider attribute or the P0_API_TOKEN environment variable. To create a token, navigate to https://p0.app/o/%s/settings.",
				model.Org.ValueString(),
			),
		)
	}
	return token
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

	api_token := resolveApiToken(ctx, model, &resp.Diagnostics)

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
		routingrules.NewRoutingRule,
		routingrules.NewRoutingRules,
		installaws.NewAwsIamWrite,
		installaws.NewIamWriteStagedAws,
		installaws.NewAwsInventory,
		installaws.NewAwsInventoryStaged,
		installk8s.NewAwsKubernetes,
		installk8s.NewKubernetesStagedAws,
		installazure.NewAzure,
		installazure.NewAzureApp,
		installazure.NewAzureAppStaged,
		installazure.NewAzureBastionHost,
		installazure.NewAzureBastionHostStaged,
		installazure.NewAzureIamWrite,
		installazure.NewAzureIamWriteStaged,
		installgcp.NewGcp,
		installgcp.NewGcpAccessLogs,
		installgcp.NewGcpIamAssessment,
		installgcp.NewGcpIamAssessmentStaged,
		installgcp.NewGcpIamWrite,
		installgcp.NewGcpIamWriteStaged,
		installgcp.NewGcpOrgAccessLogs,
		installgcp.NewGcpOrgIamAssessment,
		installgcp.NewGcpSecurityPerimeter,
		installgcp.NewGcpSecurityPerimeterStage,
		installgcp.NewGcpSharingRestriction,
		installssh.NewSshAwsIamWrite,
		installssh.NewSshGcpIamWrite,
		installssh.NewSshAzureIamWrite,
		installmysql.NewMysqlIamWriteStaged,
		installmysql.NewMysqlIamWrite,
		installpostgres.NewPostgresIamWriteStaged,
		installpostgres.NewPostgresIamWrite,
		installokta.NewOktaDirectoryListingStaged,
		installokta.NewOktaDirectoryListing,
		installokta.NewOktaGroupAssignment,
		installrds.NewRdsIamWrite,
		installsplunk.NewAuditLogs,
		installdatadog.NewAuditLogs,
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
