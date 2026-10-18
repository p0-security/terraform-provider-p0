package installtailscale

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// copy of internal/provider/resources/install/auth0/tenant.go @ 663ab81 (INT-590),
// renamed for Tailscale (INT-686): keyed by the Tailnet ID, with the credential as one
// attribute, and the mode. Its connector, credential and mode are refused at plan time
// rather than replaced, as Harbor refuses a new `url`
// (internal/provider/resources/install/harbor/instance.go).

var _ resource.Resource = &TailscaleTailnet{}
var _ resource.ResourceWithConfigure = &TailscaleTailnet{}
var _ resource.ResourceWithImportState = &TailscaleTailnet{}
var _ resource.ResourceWithModifyPlan = &TailscaleTailnet{}
var _ resource.ResourceWithValidateConfig = &TailscaleTailnet{}

// The tailnet's credential types and modes: the options of the app's
// `TailscaleCredential` and `TailscaleMode` select elements.
const (
	federatedCredential   = "federated"
	oauthClientCredential = "oauth-client"

	directWriteMode = "direct-write"
	postureMode     = "posture"
)

var (
	credentialTypes = []string{federatedCredential, oauthClientCredential}
	modes           = []string{directWriteMode, postureMode}
)

var credentialPath = path.Root("credential")

type TailscaleTailnet struct {
	installer *common.Install
}

type tailscaleTailnetModel struct {
	Id            types.String     `tfsdk:"id"`
	Connector     types.String     `tfsdk:"connector"`
	Credential    *credentialModel `tfsdk:"credential"`
	Mode          types.String     `tfsdk:"mode"`
	Discovery     types.String     `tfsdk:"discovery"`
	SuggestedMode types.String     `tfsdk:"suggested_mode"`
	State         types.String     `tfsdk:"state"`
}

type credentialModel struct {
	Type             types.String `tfsdk:"type"`
	ClientId         types.String `tfsdk:"client_id"`
	Vault            *vaultModel  `tfsdk:"vault"`
	ClientSecretName types.String `tfsdk:"client_secret_name"`
}

// The credential as ValidateConfig reads it, where the vault may be unknown, which a
// *vaultModel can't hold.
type credentialConfigModel struct {
	Type             types.String `tfsdk:"type"`
	ClientId         types.String `tfsdk:"client_id"`
	Vault            types.Object `tfsdk:"vault"`
	ClientSecretName types.String `tfsdk:"client_secret_name"`
}

type tailscaleTailnetJson struct {
	Connector     *string         `json:"connector,omitempty"`
	Credential    *credentialJson `json:"credential,omitempty"`
	Mode          *selectJson     `json:"mode,omitempty"`
	Discovery     *string         `json:"discovery,omitempty"`
	SuggestedMode *string         `json:"suggestedMode,omitempty"`
	State         *string         `json:"state,omitempty"`
}

// The JSON is a discriminated union on "type": a federated identity carries its client
// ID, and an OAuth client also where its secret is.
type credentialJson struct {
	Type             string     `json:"type"`
	ClientId         *string    `json:"clientId,omitempty"`
	Vault            *vaultJson `json:"vault,omitempty"`
	ClientSecretName *string    `json:"clientSecretName,omitempty"`
}

// A select element whose options have no fields, such as the mode: P0 stores the option
// as its "type".
type selectJson struct {
	Type string `json:"type"`
}

type tailscaleTailnetApi struct {
	Item *tailscaleTailnetJson `json:"item"`
}

func NewTailscaleTailnet() resource.Resource {
	return &TailscaleTailnet{}
}

func (*TailscaleTailnet) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_tailscale_tailnet"
}

func (*TailscaleTailnet) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A tailnet that P0 can grant network access in, through one of your Tailscale connectors. Add what requestors can reach on it with ` + "`p0_tailscale_destination`" + ` resources.

The connector acts on the tailnet with a Tailscale credential that P0 never holds:

- ` + "`federated`" + ` (the default in P0's installer): a Tailscale federated identity that trusts the connector's own cloud identity, so no secret is stored anywhere. Create it with ` + "`tailscale_federated_identity`" + `, with the connector's cloud as its issuer and the connector's identity as its subject: on AWS, the issuer of AWS outbound identity federation for the connector's account and the connector's role ARN; on Google Cloud, ` + "`https://accounts.google.com`" + ` and the connector's service account's unique numeric ID, not its email. Leave its ` + "`audience`" + ` unset: P0 uses Tailscale's default, ` + "`api.tailscale.com/<client ID>`" + `. On AWS, the connector's role also needs ` + "`sts:GetWebIdentityToken`" + ` for Tailscale's audiences, ` + "`api.tailscale.com/*`" + `, for 300 seconds or less, which the connector's Terraform grants.
- ` + "`oauth-client`" + `: a Tailscale OAuth client whose secret is in your secret manager, in the connector's cloud. Create the client in Tailscale's admin console and store its secret with your cloud's CLI, as the example does, so the secret never enters Terraform's state. The connector reads the secret itself, so let its identity read that one secret and no other.

The credential needs these scopes, by mode. Direct write: ` + "`policy_file`" + `, ` + "`users:read`" + `, ` + "`devices:core:read`" + `, ` + "`devices:posture_attributes`" + `, ` + "`devices:routes:read`" + ` and ` + "`services:read`" + `. Posture: the same, with ` + "`policy_file:read`" + ` in place of ` + "`policy_file`" + `.

The mode is how P0 grants access. In ` + "`direct-write`" + ` mode P0 edits your policy file: each grant adds a group with the requestor in it, and the grant's entries. In ` + "`posture`" + ` mode your policy file gates grants on P0 postures, and P0 only sets device attributes. ` + "`tailscale_acl`" + ` overwrites the whole policy file on every apply, which would erase P0's grants in a direct-write tailnet, so manage the policy file with ` + "`tailscale_acl`" + ` only in posture mode, with "prevent edits in the admin console" on, which ` + "`tailscale_tailnet_settings`" + ` sets as ` + "`acls_externally_managed_on`" + `. A posture destination must then depend on the ` + "`tailscale_acl`" + ` that defines its posture.

**Important:** Create the credential, and let the connector use it, before Terraform creates this resource: in an earlier apply, or in the same apply with this resource depending on the grants, as the examples do. Creating this resource has P0 validate the install through the connector: the connector gets a token with the credential, checks its scopes for the mode, checks that the mode suits the policy file, and counts what the tailnet offers. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the connector is denied right after you grant it, apply again.

P0 fixes a tailnet's ` + "`connector`" + `, ` + "`credential`" + ` and ` + "`mode`" + ` when it adds the tailnet, and its open grants were made through them. Replacing the tailnet would remove it from P0 without revoking those grants, so Terraform refuses a change to any of them at plan time. To change one, revoke the tailnet's grants, then remove this resource, apply, and add it again. A new ` + "`id`" + ` is another tailnet, and replaces this one. Destroying this resource removes the tailnet from P0 and revokes nothing, so revoke its grants first. If the tailnet is already added in P0, import it rather than creating it: P0 refuses to add a tailnet twice. If P0 hasn't finished installing the tailnet, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.

**Prerequisites:**

- A Tailscale connector, installed with ` + "`p0_tailscale_connector`" + `. An OAuth client's secret must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.

- For a federated identity on AWS, AWS outbound identity federation turned on for the connector's account.

- For an OAuth client, P0's secret manager integration for where the secret is: AWS Secrets Manager (` + "`p0_aws_secrets_manager`" + `) for the AWS account, or Google Secret Manager for the Google Cloud project.

**Note:** This integration is currently in beta.

For the full install steps, see [Tailscale](https://docs.p0.dev/integrations/resource-integrations/tailscale).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The Tailnet ID from Tailscale's Settings, General, such as `T1234CNTRL`: the `tailnet` your `tailscale` provider is configured with. " +
					"A tailnet created before October 2025 also answers to its legacy ID, such as `example.com`. " +
					"The tailnet's DNS name, such as `tail1234.ts.net`, can't be used: Tailscale's API doesn't take it. " +
					"Changing it replaces the installation, which removes the tailnet from P0 and revokes nothing, so revoke the tailnet's grants first.",
				Validators: []validator.String{tailnetIdValidator()},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"connector": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The connector that acts on this tailnet: the `id` of a `p0_tailscale_connector` resource. Requires that connector to be installed. It can't change.",
				Validators:          connectorIdValidators(),
			},
			"credential": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "The Tailscale credential the connector uses for this tailnet. P0 never holds a secret for it. It can't change.",
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "How the connector authenticates to Tailscale: either `federated` (a federated identity that trusts the connector's own cloud identity) or `oauth-client` (an OAuth client whose secret is in your secret manager)",
						Validators: []validator.String{
							stringvalidator.OneOf(credentialTypes...),
						},
					},
					"client_id": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "The client ID of the federated identity or OAuth client, as Tailscale's Trust credentials page shows it, such as `kAbc1CNTRL`. For a federated identity, the `id` of its `tailscale_federated_identity`.",
					},
					"vault": vaultAttribute(
						"The secret manager that holds the OAuth client's secret. Required for, and only valid with, `oauth-client`. Your connector reads the secret, and P0 never does. It must be in the connector's cloud, and can be in another account or project there.",
						secretPlace.what,
					),
					"client_secret_name": schema.StringAttribute{
						Optional: true,
						MarkdownDescription: `The name of the secret that holds, or will hold, the OAuth client's secret, such as ` + "`tailscale/oauth-client`" + ` in AWS Secrets Manager or ` + "`tailscale-oauth-client`" + ` in Google Secret Manager: the name alone, not the secret's ARN or its full resource name. Required for, and only valid with, ` + "`oauth-client`" + `. ` +
							`In AWS Secrets Manager the name can't end in a hyphen and six letters or digits, like ` + "`-AbCdEf`" + `, because Secrets Manager can't find a secret by such a name. ` +
							`To rotate the secret, store the new one as a new version of the same secret, which needs no change here.`,
					},
				},
			},
			"mode": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "How P0 grants access on this tailnet: either `direct-write` (P0 edits your policy file) or `posture` (your policy file gates grants on P0 postures, and P0 only sets device attributes). " +
					"Use `posture` when \"prevent edits in the admin console\" is on, such as when a GitOps pipeline or `tailscale_acl` owns the policy file. It can't change.",
				Validators: []validator.String{
					stringvalidator.OneOf(modes...),
				},
			},
			"discovery": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "What P0 found on the tailnet when it last validated it, such as its tags, subnet routes and groups. Direct-write mode adds one group per open grant, and Tailscale's plans cap a tailnet's groups.",
			},
			"suggested_mode": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The mode that suits the tailnet's policy file, as P0 found it when it last validated the tailnet: `posture` when \"prevent edits in the admin console\" is on, `direct-write` otherwise.",
			},
			"state": common.StateAttribute,
		},
	}
}

// Checking here rather than in Create reports a mistake at plan time, before the
// provider calls P0, in P0's words. A value that Terraform only knows at apply time
// is skipped while planning, and checked when Terraform validates the configuration
// again before it applies.
func (*TailscaleTailnet) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var credential credentialConfigModel
	if !common.GetKnownObject(ctx, req.Config, credentialPath, &credential, &resp.Diagnostics) {
		return
	}
	vault := vaultFromConfig(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	validateClientId(credential.ClientId, &resp.Diagnostics)
	validateCredentialFields(&credential, &resp.Diagnostics)
	if credential.Type.IsUnknown() || credential.Type.ValueString() == oauthClientCredential {
		validateClientSecret(vault, credential.ClientSecretName, &resp.Diagnostics)
	}
}

// Rejects a credential whose vault and secret name don't match its type: an OAuth
// client needs both, and a federated identity has neither. A field that Terraform
// doesn't know yet is neither missing nor unexpected.
func validateCredentialFields(credential *credentialConfigModel, diags *diag.Diagnostics) {
	vaultPresent := !credential.Vault.IsNull() && !credential.Vault.IsUnknown()
	switch credential.Type.ValueString() {
	case oauthClientCredential:
		if credential.Vault.IsNull() {
			diags.AddAttributeError(vaultPath, "Missing vault for the OAuth client's secret",
				fmt.Sprintf("'%s' is required when '%s' is %q.", vaultPath, credentialPath.AtName("type"), oauthClientCredential))
		}
		if credential.ClientSecretName.IsNull() {
			diags.AddAttributeError(clientSecretNamePath, "Missing secret name for the OAuth client's secret",
				fmt.Sprintf("'%s' is required when '%s' is %q.", clientSecretNamePath, credentialPath.AtName("type"), oauthClientCredential))
		}
	case federatedCredential:
		if vaultPresent {
			diags.AddAttributeError(vaultPath, "Unexpected vault",
				fmt.Sprintf("'%s' may only be set when '%s' is %q.", vaultPath, credentialPath.AtName("type"), oauthClientCredential))
		}
		if isSet(credential.ClientSecretName) {
			diags.AddAttributeError(clientSecretNamePath, "Unexpected secret name",
				fmt.Sprintf("'%s' may only be set when '%s' is %q.", clientSecretNamePath, credentialPath.AtName("type"), oauthClientCredential))
		}
	}
}

// P0's message for a client ID it refuses (INVALID_CLIENT_ID). It leaves the value out,
// because a customer may have pasted the OAuth client's secret in its place.
const invalidClientId = "Enter the client ID of the Tailscale federated identity or OAuth client, as Tailscale's Trust credentials page shows it."

// Tailscale's rule for a client ID: letters, digits, `_` and `-`, 1 to 128 characters
// (TAILSCALE_CLIENT_ID_PATTERN).
var clientIdPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// P0's check (`validateClientId` in
// packages/integrations/tailscale/backend/src/install/validation.ts in the app). Like
// P0, it checks the ID without the whitespace around it, and never repeats it.
func validateClientId(clientId types.String, diags *diag.Diagnostics) {
	validateField(credentialPath.AtName("client_id"), clientId, []rule{
		notTailscaleSecret("Tailscale secret entered as the client ID", tailscaleSecretAsClientId),
		{
			summary: "Invalid client ID",
			message: invalidClientId,
			test:    clientIdPattern.MatchString,
		},
	}, diags)
}

func (r *TailscaleTailnet) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        TailscaleKey,
		Component:          tailnetComponent,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        r.getItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeCheckError("tailnet"),
	}
}

func (r *TailscaleTailnet) getId(data any) *string {
	model, ok := data.(*tailscaleTailnetModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

// A response with no item is a bad response, not an item to read. Its nil item,
// returned as it is, wouldn't compare equal to nil, and would be read as one.
func (r *TailscaleTailnet) getItemJson(json any) any {
	inner, ok := json.(*tailscaleTailnetApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner.Item
}

func (r *TailscaleTailnet) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*tailscaleTailnetJson)
	if !ok {
		return nil
	}

	// credential and mode are Required, so a nil one would surface as Terraform core's
	// "provider produced inconsistent result after apply", which tells the reader
	// nothing to do.
	if jsonv.Credential == nil || jsonv.Mode == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The Tailscale tailnet %s carries no credential or no mode. "+
				"Remove it in the P0 app and add it again, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return nil
	}

	// A credential, a vault or a mode of a kind this provider doesn't know can't be read
	// into the attributes.
	credential := jsonv.Credential
	switch {
	case !slices.Contains(credentialTypes, credential.Type):
		reportUnsupportedTailnet(diags, id, "credential", credential.Type, credentialTypes)
		return nil
	case !slices.Contains(modes, jsonv.Mode.Type):
		reportUnsupportedTailnet(diags, id, "mode", jsonv.Mode.Type, modes)
		return nil
	case credential.Type == oauthClientCredential && credential.Vault == nil:
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The Tailscale tailnet %s has an OAuth client credential with no vault. "+
				"Remove it in the P0 app and add it again, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return nil
	case credential.Vault != nil && !slices.Contains(vaultTypes, credential.Vault.Type):
		reportUnsupportedTailnet(diags, id, "vault", credential.Vault.Type, vaultTypes)
		return nil
	}

	model := &credentialModel{
		Type:             types.StringValue(credential.Type),
		ClientId:         types.StringPointerValue(credential.ClientId),
		ClientSecretName: types.StringPointerValue(credential.ClientSecretName),
	}
	if credential.Vault != nil {
		model.Vault = vaultFromJson(credential.Vault)
	}

	return &tailscaleTailnetModel{
		Id:            types.StringValue(id),
		Connector:     types.StringPointerValue(jsonv.Connector),
		Credential:    model,
		Mode:          types.StringValue(jsonv.Mode.Type),
		Discovery:     types.StringPointerValue(jsonv.Discovery),
		SuggestedMode: types.StringPointerValue(jsonv.SuggestedMode),
		State:         types.StringPointerValue(jsonv.State),
	}
}

func reportUnsupportedTailnet(diags *diag.Diagnostics, id string, what string, got string, supported []string) {
	diags.AddError(
		"Unsupported Tailscale tailnet",
		fmt.Sprintf("The Tailscale tailnet %s has a %q %s. This provider supports only %s. "+
			"Upgrade the provider, manage the tailnet in the P0 app, or remove it from Terraform state with `terraform state rm`.",
			id, got, what, strings.Join(supported, " and ")),
	)
}

// The body of the PUT that stages the item, and of verify and configure: the
// connector, the credential and the mode, which P0 fixes when it creates the item.
// Sending them on each step has P0 refuse a request that would change any of them,
// before it calls the connector. P0 fills in what discovery finds, and sets the state.
func (r *TailscaleTailnet) toJson(data any) any {
	datav, ok := data.(*tailscaleTailnetModel)
	if !ok {
		return nil
	}

	json := tailscaleTailnetJson{
		Connector: datav.Connector.ValueStringPointer(),
		Mode:      &selectJson{Type: datav.Mode.ValueString()},
	}
	if credential := datav.Credential; credential != nil {
		json.Credential = &credentialJson{
			Type:             credential.Type.ValueString(),
			ClientId:         credential.ClientId.ValueStringPointer(),
			ClientSecretName: credential.ClientSecretName.ValueStringPointer(),
		}
		if credential.Vault != nil {
			json.Credential.Vault = credential.Vault.toJson()
		}
	}
	return &json
}

// P0 checks every field before it creates the item, without repeating the value, which
// may be the OAuth client's secret pasted in place of the secret's name or the client
// ID. So a value it refuses leaves nothing staged. A check that fails after that leaves
// the staged item in state, tainted, and the next apply replaces it, because P0 refuses
// a second PUT of a tailnet that's already added.
func (r *TailscaleTailnet) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json tailscaleTailnetApi
	var data tailscaleTailnetModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *TailscaleTailnet) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &tailscaleTailnetApi{}, &tailscaleTailnetModel{})
}

// Refuses a new connector, credential or mode for an installed tailnet, and reports
// whether it did. Only an installed tailnet can have open grants. For one imported
// before it finished installing, P0 refuses the change itself. A value that Terraform
// doesn't know yet is checked again in Update, when it is. A new ID replaces the
// tailnet with another, so it goes through.
//
// Replacing the tailnet under its ID, which is what Terraform would do for a new
// value, deletes the item and creates one with the same ID. P0 revokes nothing when
// it deletes the item, so the grants made through the old connector, credential or
// mode would stay open.
//
// The credential is compared whole, so a vault that goes from set to null is a change
// too: the framework runs no plan modifier on the fields of a nested attribute whose
// planned value is null.
func refuseFixedChanges(ctx context.Context, state tfsdk.State, plan tfsdk.Plan, diags *diag.Diagnostics) bool {
	var installState, idBefore, idAfter, connectorBefore, connectorAfter, modeBefore, modeAfter types.String
	var credentialBefore, credentialAfter types.Object
	diags.Append(state.GetAttribute(ctx, path.Root("state"), &installState)...)
	diags.Append(state.GetAttribute(ctx, path.Root("id"), &idBefore)...)
	diags.Append(plan.GetAttribute(ctx, path.Root("id"), &idAfter)...)
	diags.Append(state.GetAttribute(ctx, path.Root("connector"), &connectorBefore)...)
	diags.Append(plan.GetAttribute(ctx, path.Root("connector"), &connectorAfter)...)
	diags.Append(state.GetAttribute(ctx, credentialPath, &credentialBefore)...)
	diags.Append(plan.GetAttribute(ctx, credentialPath, &credentialAfter)...)
	diags.Append(state.GetAttribute(ctx, path.Root("mode"), &modeBefore)...)
	diags.Append(plan.GetAttribute(ctx, path.Root("mode"), &modeAfter)...)
	if diags.HasError() || installState.ValueString() != common.StateInstalled || !idBefore.Equal(idAfter) {
		return false
	}

	refused := false
	for _, fixed := range []struct {
		name          string
		before, after attr.Value
	}{
		{name: "connector", before: connectorBefore, after: connectorAfter},
		{name: "credential", before: credentialBefore, after: credentialAfter},
		{name: "mode", before: modeBefore, after: modeAfter},
	} {
		if !changed(ctx, fixed.before, fixed.after) {
			continue
		}
		diags.AddAttributeError(
			path.Root(fixed.name),
			fmt.Sprintf("A tailnet's %s can't change", fixed.name),
			fmt.Sprintf("P0 fixes a tailnet's %s when it adds the tailnet. Replacing the tailnet to change it would remove it from P0 without revoking its open grants. "+
				"To change it, revoke the tailnet's grants, then remove this p0_tailscale_tailnet, apply, and add it again.", fixed.name),
		)
		refused = true
	}
	return refused
}

// Whether after, once Terraform knows all of it, differs from before.
func changed(ctx context.Context, before, after attr.Value) bool {
	value, err := after.ToTerraformValue(ctx)
	if err != nil || !value.IsFullyKnown() {
		return false
	}
	return !before.Equal(after)
}

// Refuses a change to what P0 fixes, and plans an update for a tailnet that P0 hasn't
// installed, so that the next apply finishes the install. P0 fills in what discovery
// finds during that apply, so it stays unknown until then.
func (*TailscaleTailnet) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Nothing is installed yet on create, and nothing is left to finish on destroy.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	if refuseFixedChanges(ctx, req.State, req.Plan, &resp.Diagnostics) || resp.Diagnostics.HasError() {
		return
	}
	planUnfinishedInstall(ctx, req, resp, path.Root("discovery"), path.Root("suggested_mode"))
}

// Terraform plans an update in two cases. ModifyPlan plans one to finish the install of
// an item that isn't installed, which verifies and configures the item as Create does:
// P0 validates the install on verify alone, so there is no shorter way. And a
// connector, credential or mode that Terraform only knows at apply time plans one,
// which is refused here if the value turns out to be a new one.
func (r *TailscaleTailnet) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if refuseFixedChanges(ctx, req.State, req.Plan, &resp.Diagnostics) || resp.Diagnostics.HasError() {
		return
	}
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &tailscaleTailnetApi{}, &tailscaleTailnetModel{})
}

func (r *TailscaleTailnet) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &tailscaleTailnetModel{})
}

// Imports a tailnet by its Tailnet ID, once the ID passes the check that the
// configuration's gets. The read that follows the import puts the ID in a URL path,
// where `T1234#x` reads `T1234`, and the provider's client fails on a path that
// doesn't parse, such as one with `%zz`.
func (r *TailscaleTailnet) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !isTailnetId(req.ID) {
		resp.Diagnostics.AddError("Invalid Tailnet ID", invalidTailnetId)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
