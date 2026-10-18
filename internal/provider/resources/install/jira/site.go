package installjira

import (
	"context"
	"fmt"
	"net/http"
	"unicode/utf16"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

var _ resource.Resource = &Site{}
var _ resource.ResourceWithConfigure = &Site{}
var _ resource.ResourceWithImportState = &Site{}
var _ resource.ResourceWithModifyPlan = &Site{}
var _ resource.ResourceWithValidateConfig = &Site{}

// The Jira `site` item: one Jira Cloud site, the connector deployment that reaches
// it, and where that connector finds the site's OAuth client secret
// (packages/integrations/jira/shared/src/components.ts in the app).
type Site struct {
	installer *common.Install
}

type siteModel struct {
	Site             types.String                                        `tfsdk:"site"`
	Hosting          *installapp.ConnectorHostingWithServiceAccountModel `tfsdk:"hosting"`
	Vault            *vaultModel                                         `tfsdk:"vault"`
	ClientSecretName types.String                                        `tfsdk:"client_secret_name"`
	DomainPattern    types.String                                        `tfsdk:"domain_pattern"`
	ClientId         types.String                                        `tfsdk:"client_id"`
	CloudId          types.String                                        `tfsdk:"cloud_id"`
	State            types.String                                        `tfsdk:"state"`
}

// The item, as P0 stores it.
type siteJson struct {
	Hosting          *installapp.ConnectorHostingJson `json:"hosting,omitempty"`
	Vault            *vaultJson                       `json:"vault,omitempty"`
	ClientSecretName *string                          `json:"clientSecretName,omitempty"`
	DomainPattern    *string                          `json:"domainPattern,omitempty"`
	ClientId         *string                          `json:"clientId,omitempty"`
	CloudId          *string                          `json:"cloudId,omitempty"`
	State            *string                          `json:"state,omitempty"`
}

type siteApi struct {
	Item *siteJson `json:"item"`
}

func NewSite() resource.Resource {
	return &Site{}
}

func (*Site) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_jira"
}

func (*Site) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Jira installation for one Jira Cloud site.

Installing Jira lets P0 grant your users just-in-time roles on the site's company-managed projects. P0 acts through a connector that you deploy in your own AWS account or Google Cloud project, one for each site. The connector signs in to Jira as an Atlassian service account, with an OAuth 2.0 credential whose client secret stays in your secret manager: P0 never reads it, and only the connector does.

**Important:** Create the service account's OAuth credential and store its client secret before you apply this resource, and deploy the connector first: in an earlier apply, or in the same one with this resource depending on the connector's access grants, as the examples do. Creating this resource has P0 check the site through the connector: that P0 can invoke the connector, that the connector can read the client secret and sign in to the site with it, and that the service account holds the Jira permissions P0 uses. If a check fails, the apply fails with P0's message.

Changing ` + "`site`" + `, ` + "`hosting`" + `, ` + "`vault`" + `, ` + "`client_secret_name`" + ` or ` + "`domain_pattern`" + ` replaces the installation. To rotate the client secret, add a new version to the same secret, which needs no change here. Changing ` + "`client_id`" + ` updates the installation in place, and P0 checks the site again. If the check fails, the apply fails, and an installed site keeps its current client ID. An installation that P0 hasn't finished, such as one imported before its checks passed, plans an update, and applying it finishes the install.

` + "`domain_pattern`" + ` is also the connector's ` + "`DOMAIN_PATTERN`" + ` environment variable, and P0 checks that the two are the same whenever it checks the site. To change the pattern, change both in one apply: the connector is deployed with the new pattern, and the installation is replaced with it.

**Warning:** Replacing or destroying this resource removes the site from P0, and P0 can't revoke access on a site it no longer has. Before you change an attribute that replaces the installation, or destroy this resource, revoke the site's open access in P0 or wait for it to expire.

P0 checks the site's host, where the connector runs, the vault, the secret's name and the domain pattern when it creates the installation. A plan checks all of them by P0's rules, except whether ` + "`domain_pattern`" + ` compiles and is safe to match, which waits for P0 at apply. The connector and the secret must be in the same cloud, and on AWS in commercial regions: Jira doesn't support AWS GovCloud, China, ISO or European Sovereign Cloud regions.

**Prerequisites:**

- An Atlassian service account in the organization that owns the site, with access to Jira on the site. Add it to a group that holds the Jira global permissions Administer Jira, to change project roles, and Browse users and groups, to find people by their email. Give it an OAuth 2.0 credential with the scopes ` + "`manage:jira-configuration`" + `, ` + "`manage:jira-project`" + `, ` + "`read:jira-user`" + ` and ` + "`read:jira-work`" + `, and store the credential's client secret as a secret in the secret manager that ` + "`vault`" + ` names.

  **Warning:** Administer Jira lets the service account change every project's roles, and the site's configuration.

- P0's Jira connector, deployed where ` + "`hosting`" + ` says, with its ` + "`DOMAIN_PATTERN`" + ` environment variable set to ` + "`domain_pattern`" + `. P0's Jira installer generates Terraform that deploys the connector with its image pinned, as in the examples.

  Grant the connector read access to the client secret alone, never through a wildcard that matches other secrets, or at the account or project level. P0 names the secret in each call, so any other secret the connector can read is one a call could point it at.

- For AWS Lambda, the image ` + "`p0security/p0-connector-jira`" + `, running as an IAM role named ` + "`hosting.connector_name`" + `. Lambda runs images only from Amazon ECR, so copy the image from Docker Hub into ECR first. Give the role ` + "`secretsmanager:GetSecretValue`" + ` on ` + "`arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<client_secret_name>-??????`" + `, with the vault's region and account, and ` + "`kms:Decrypt`" + ` on the secret's key if a customer-managed KMS key encrypts it. A secret in an account other than the connector's needs a customer-managed key, and the secret's resource policy and the key's policy must allow the connector's role. ` + "`p0_aws_iam_write`" + ` must be installed for the account the connector runs in. Grant that installation's role ` + "`lambda:InvokeFunction`" + ` on the connector's function.

- For Google Cloud Run, the image ` + "`p0security/p0-connector-jira-gcloud`" + `, running as ` + "`hosting.connector_service_account`" + `: the service account whose ID is ` + "`hosting.connector_name`" + `, in the connector's project. Give it ` + "`roles/secretmanager.secretAccessor`" + ` and ` + "`roles/secretmanager.viewer`" + ` on the secret alone. ` + "`p0_gcp`" + ` must be installed. Grant its service account ` + "`roles/run.invoker`" + ` and ` + "`roles/run.viewer`" + ` on the connector's service, and pass that service account's email to the connector as its ` + "`INVOKER_SA_EMAIL`" + ` environment variable.

**Note:** This integration is in beta.

For the full install steps, see [Jira](https://docs.p0.dev/integrations/resource-integrations/jira).`,
		Attributes: map[string]schema.Attribute{
			"site": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The site's host, in lowercase, such as `acme.atlassian.net`: `<name>.atlassian.net`, without `https://` or anything after the host.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"hosting": hostingAttribute(),
			"vault":   vaultAttribute(),
			// P0 fixes the client secret's secret when it creates the item, and refuses a
			// change to it, so a new secret replaces the installation.
			"client_secret_name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The name of the secret that holds the service account's OAuth client secret, in `vault`, such as `p0/jira/acme`: the name alone, never an ARN or a resource name. " +
					"For `aws-sm`, up to 512 letters, digits and `/_+=.@-`, and, when the vault's account isn't the connector's, not ending in a hyphen and six letters or digits, like `-AbCdEf`. " +
					"For `gcp-sm`, up to 255 letters, digits, underscores and hyphens. " +
					"Changing it replaces the installation. To rotate the client secret, add a new version to the same secret instead.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			// P0 fixes the pattern when it creates the item: the connector's Terraform is
			// rendered from it.
			"domain_pattern": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "A regular expression that the connector matches against the whole email domain of every user it grants to, such as `example\\.com`. The connector refuses any other user. " +
					"It is the connector's `DOMAIN_PATTERN` environment variable too, and P0 checks that the two are the same. " +
					"P0 refuses a pattern that doesn't compile, or one it can't show won't backtrack catastrophically: allow subdomains with `(.+\\.)?example\\.com` rather than `(.+\\.)*example\\.com`. " +
					"Changing it replaces the installation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			// client_id can change in place, for a new credential, so it doesn't require
			// replacement. P0 checks the site again when it changes.
			"client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The client ID of the service account's OAuth 2.0 credential. Changing it updates the installation in place, and P0 checks the site again.",
			},
			// A site's host always has the same cloud ID, so it is known in the plan of an
			// update.
			"cloud_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The site's Atlassian cloud ID, which P0 finds through the connector when it checks the site",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"state": common.StateAttribute,
		},
	}
}

// Validating here rather than in Create surfaces mistakes at plan time, before P0 is
// called, in P0's words. A value that Terraform only knows at apply time is skipped
// while planning, and checked when Terraform validates the configuration again before
// it applies.
func (*Site) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var site, secretName, domainPattern, clientId types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("site"), &site)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("client_secret_name"), &secretName)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("domain_pattern"), &domainPattern)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("client_id"), &clientId)...)
	vault := vaultFromConfig(ctx, req.Config, &resp.Diagnostics)
	var hosting *installapp.ConnectorHostingModel
	if withAccount := installapp.ConnectorHostingWithServiceAccountFromConfig(ctx, req.Config, &resp.Diagnostics); withAccount != nil {
		hosting = &withAccount.ConnectorHostingModel
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// In the order P0 checks them when it creates the item.
	validateSiteHost(site, &resp.Diagnostics)
	validateHosting(hosting, &resp.Diagnostics)
	validateVault(vault, &resp.Diagnostics)
	validateVaultCloud(vault, hosting, &resp.Diagnostics)
	validateSecretName(secretName, vault, hosting, &resp.Diagnostics)
	validateField(path.Root("domain_pattern"), domainPattern, domainPatternRules, &resp.Diagnostics)
	validateField(path.Root("client_id"), clientId, clientIdRules, &resp.Diagnostics)
}

// P0's rules for the domain pattern that Go can check, and its messages for a pattern
// that breaks one (packages/integrations/jira/shared/src/domain-pattern.ts in the
// app). P0 also refuses a pattern that doesn't compile as a JavaScript regular
// expression, or that redos-detector can't show is safe to match. Go's regular
// expressions aren't JavaScript's, so those are left to P0.
const (
	domainPatternMaxLength = 1000
	missingDomainPattern   = "Allowed email domains are required"
)

var longDomainPattern = fmt.Sprintf("Allowed email domains must be at most %d characters", domainPatternMaxLength)

var domainPatternRules = []rule{
	{summary: "Missing domain pattern", message: missingDomainPattern, test: func(pattern string) bool { return pattern != "" }},
	// JavaScript counts a string's length in UTF-16 code units.
	{summary: "Domain pattern too long", message: longDomainPattern, test: func(pattern string) bool {
		return len(utf16.Encode([]rune(pattern))) <= domainPatternMaxLength
	}},
}

// P0's rules for the OAuth client's ID, and its messages for one that breaks them
// (validateClientId in packages/integrations/jira/backend/src/install/site.ts in the
// app): printable ASCII without whitespace, up to 256 characters. Atlassian's are 32.
const (
	clientIdMaxLength = 256
	missingClientId   = "Enter the client ID of the service account's OAuth credential."
	invalidClientId   = "That doesn't look like an OAuth client ID. Enter the client ID from the service account's OAuth credential in Atlassian Administration, not its secret."
)

var clientIdRules = []rule{
	{summary: "Missing client ID", message: missingClientId, test: func(id string) bool { return id != "" }},
	{summary: "Invalid client ID", message: invalidClientId, test: func(id string) bool {
		if len(id) > clientIdMaxLength {
			return false
		}
		for _, c := range id {
			if c < 0x21 || c > 0x7e {
				return false
			}
		}
		return true
	}},
}

func (r *Site) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        JiraKey,
		Component:          siteComponent,
		ProviderData:       data,
		GetId:              siteId,
		GetItemJson:        siteItemJson,
		FromJson:           siteFromJson,
		ToJson:             siteToJson,
		DescribeCheckError: describeCheckError,
	}
}

// P0 runs the install checks through the connector, so a failed check usually means a
// problem in the customer's setup, which P0's message describes. The wording doesn't
// presume where the problem is, and leaves that to P0's message.
func describeCheckError(site string, err error) (string, string) {
	return "Jira install check failed",
		fmt.Sprintf("P0 rejected the install check for the Jira site %q:\n\n%s", site, err)
}

func siteId(data any) *string {
	model, ok := data.(*siteModel)
	if !ok {
		return nil
	}
	str := model.Site.ValueString()
	return &str
}

func siteItemJson(json any) any {
	// An answer with no item leaves Item nil, which has to come back as an untyped
	// nil: a nil *siteJson in an any isn't nil, and would reach FromJson.
	inner, ok := json.(*siteApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner.Item
}

func siteFromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*siteJson)
	if !ok {
		return nil
	}

	// vault and hosting are Required, so leaving either nil here would surface as
	// Terraform core's "provider produced inconsistent result after apply" rather than
	// something the reader can act on.
	if jsonv.Vault == nil || jsonv.Hosting == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The Jira site %s carries no vault or hosting configuration. "+
				"Configure it in the P0 app, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return nil
	}

	vault := vaultFromJson(jsonv.Vault)
	if vault == nil || (jsonv.Hosting.Type != installapp.AwsHosting && jsonv.Hosting.Type != installapp.GcpHosting) {
		diags.AddError(
			"Unsupported Jira site",
			fmt.Sprintf("The Jira site %s has a %q vault and %q hosting. "+
				"This provider supports %q and %q vaults, and %q and %q hosting. Manage it in the P0 app, "+
				"or remove it from Terraform state with `terraform state rm`.",
				id, jsonv.Vault.Type, jsonv.Hosting.Type,
				awsSecretsManager, installvaultedcredential.GcpSecretManager, installapp.AwsHosting, installapp.GcpHosting),
		)
		return nil
	}

	return &siteModel{
		Site:             types.StringValue(id),
		Hosting:          installapp.ConnectorHostingWithServiceAccountFromJson(jsonv.Hosting),
		Vault:            vault,
		ClientSecretName: types.StringPointerValue(jsonv.ClientSecretName),
		DomainPattern:    types.StringPointerValue(jsonv.DomainPattern),
		ClientId:         types.StringPointerValue(jsonv.ClientId),
		CloudId:          types.StringPointerValue(jsonv.CloudId),
		State:            types.StringPointerValue(jsonv.State),
	}
}

// The body of configure: the whole item, but for what P0 sets. P0 compares each field
// it fixed when it created the item with the one it stored, so it refuses to install a
// site that isn't the configured one. The client ID is the one field that can change.
func siteToJson(data any) any {
	datav, ok := data.(*siteModel)
	if !ok {
		return nil
	}
	json := itemJson(datav)
	json.ClientId = datav.ClientId.ValueStringPointer()
	return json
}

// The body of the PUT that stages the item, and of verify: the whole item but the
// client ID and what P0 sets. In P0's installer the client ID is entered on the
// configure step, after the connector is deployed, and a verify checks the site through
// the connector whenever it has one: the one the request sends, or else the stored one.
// On Google Cloud the stored site has no address for the connector until a verify
// stores it, so a client ID before configure would have the first verify call a
// connector it can't address. P0 checks each field before it creates the item, so a
// value it refuses leaves nothing staged, and its error doesn't repeat the value, which
// may be the client secret pasted in place of its secret's name.
func itemJson(data *siteModel) *siteJson {
	json := siteJson{
		ClientSecretName: data.ClientSecretName.ValueStringPointer(),
		DomainPattern:    data.DomainPattern.ValueStringPointer(),
	}
	if data.Hosting != nil {
		json.Hosting = data.Hosting.ToJson()
	}
	if data.Vault != nil {
		json.Vault = data.Vault.toJson()
	}
	return &json
}

func (r *Site) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json siteApi
	var data siteModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	// A step that fails after the PUT leaves the item where it got to, at "stage" if
	// verify failed and at "configure" if configure did, and the resource tainted, so
	// the next apply deletes the item and creates it again. P0 refuses a PUT over a
	// site it has installed, or one whose fixed fields differ, so a site added in the
	// P0 app isn't taken over here: import it instead.
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, itemJson(&data))
	if resp.Diagnostics.HasError() {
		return
	}

	r.verifyAndConfigure(ctx, &req.Plan, &resp.State, &resp.Diagnostics)
}

// Verifies the item without its client ID, then configures it with the client ID, as
// P0's installer does (see itemJson). common.Install sends one body to both steps, so
// the verify is posted here, and its failures described as common.Install describes
// them.
func (r *Site) verifyAndConfigure(ctx context.Context, plan *tfsdk.Plan, state *tfsdk.State, diags *diag.Diagnostics) {
	var data siteModel
	diags.Append(plan.Get(ctx, &data)...)
	if diags.HasError() {
		return
	}

	site := data.Site.ValueString()
	stepPath := fmt.Sprintf("integrations/%s/config/%s/%s/%s", JiraKey, siteComponent, site, common.Verify)
	httpResp, err := r.installer.ProviderData.Post(stepPath, itemJson(&data), &siteApi{})
	if err != nil {
		status := 0
		if httpResp != nil {
			status = httpResp.StatusCode
		}
		switch status {
		case http.StatusNotFound:
			diags.AddError(fmt.Sprintf("Could not verify %s component", siteComponent),
				fmt.Sprintf("P0 has no %s item %q. It may have been removed while Terraform was applying this change. Apply again to recreate it.", siteComponent, site))
		case http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusBadGateway:
			diags.AddError(describeCheckError(site, err))
		default:
			diags.AddError("Error communicating with P0", fmt.Sprintf("Could not verify %s component, got error:\n%s", siteComponent, err))
		}
		return
	}

	r.installer.UpsertFromConfigure(ctx, diags, plan, state, &siteApi{}, &siteModel{})
}

func (r *Site) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &siteApi{}, &siteModel{})
}

// Plans an update for an item that P0 hasn't installed, even when the configuration
// hasn't changed, so that the next apply finishes the install: P0 grants nothing on a
// site until its item is installed. That covers an item imported before its checks
// passed, and one that a failed apply left at configure. An installed item keeps the
// framework's plan, which shows no difference while the configuration matches it.
//
// A copy of github-repositories' ModifyPlan, which also leaves unknown what the
// update's verify and configure set: the cloud ID and, on Google Cloud, the
// connector's URL.
func (*Site) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Nothing is installed yet on create, and nothing is left to finish on destroy.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() || state.ValueString() == common.StateInstalled {
		return
	}

	// As in any update the framework plans, what only P0 sets is unknown until apply.
	// Update takes it from P0's response.
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("state"), types.StringUnknown())...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("cloud_id"), types.StringUnknown())...)

	var hosting types.Object
	resp.Diagnostics.Append(resp.Plan.GetAttribute(ctx, path.Root("hosting"), &hosting)...)
	if resp.Diagnostics.HasError() || hosting.IsNull() || hosting.IsUnknown() {
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("hosting").AtName("connector_service_uri"), types.StringUnknown())...)
}

// Only client_id updates in place, and P0 checks the site with it. An item that P0 has
// verified, at configure or installed, gets the configure step alone: P0 checks the
// new client ID and saves nothing if the check fails, so a failed update keeps the
// current one. Any other item, such as a staged one, is verified first, as Create
// does.
func (r *Site) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	switch state.ValueString() {
	case common.StateConfigure, common.StateInstalled:
		r.installer.UpsertFromConfigure(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &siteApi{}, &siteModel{})
	default:
		r.verifyAndConfigure(ctx, &req.Plan, &resp.State, &resp.Diagnostics)
	}
}

func (r *Site) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &siteModel{})
}

// Imports a site by its host, once the host passes P0's rule. The read that follows
// an import puts the host in a URL path as it is, where one with a "/" or a "#" would
// read some other path. The host is quoted, so that whitespace around it shows.
func (r *Site) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !siteHostRegex.MatchString(req.ID) {
		resp.Diagnostics.AddAttributeError(path.Root("site"), invalidSiteHostSummary, fmt.Sprintf("%s Got: %q", invalidSiteHost, req.ID))
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("site"), req, resp)
}
