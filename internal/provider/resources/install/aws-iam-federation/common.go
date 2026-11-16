package installawsiamfederation

import (
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
)

const AwsIamFederationKey = "aws-iam-federation"

const defaultPoolSize = 10

// FEDERATION_ID_PATTERN in the app. The ID is part of IAM names, which have at most
// 64 characters.
var federationIdRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

const docsLink = `For the AWS install steps, see [Setting up AWS IAM management](https://docs.p0.dev/integrations/resource-integrations/aws/installation-methods).`

type providerJson struct {
	Type string `json:"type"`
	App  string `json:"app"`
}

type federationJson struct {
	State        *string                  `json:"state,omitempty"`
	AccountId    *string                  `json:"accountId,omitempty"`
	AwsPartition *installaws.AwsPartition `json:"awsPartition,omitempty"`
	PoolSize     *string                  `json:"poolSize,omitempty"`
	Provider     *providerJson            `json:"provider,omitempty"`
}

// The configuration that the staged and the final resource share. P0 fixes each of
// these when it adds the item, so a change replaces the item.
type federationConfigModel struct {
	Id        types.String `tfsdk:"id"`
	AccountId types.String `tfsdk:"account_id"`
	Partition types.String `tfsdk:"partition"`
	PoolSize  types.Int64  `tfsdk:"pool_size"`
	Provider  types.String `tfsdk:"identity_provider"`
	App       types.String `tfsdk:"app"`
}

func configAttributes() map[string]schema.Attribute {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: `A name you choose for the federation, such as ` + "`okta-prod`" + `. P0 names the federation's IAM role, SAML provider and pool path after it. Use only lowercase letters, digits, and hyphens, start with a letter or a digit, and use at most 32 characters.`,
			Validators: []validator.String{
				stringvalidator.RegexMatches(federationIdRegex, "Use only lowercase letters, digits, and hyphens, start with a letter or a digit, and use at most 32 characters"),
			},
			PlanModifiers: replace,
		},
		"account_id": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: `The ID of the AWS account that users sign in to`,
			Validators: []validator.String{
				stringvalidator.RegexMatches(installaws.AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits"),
			},
			PlanModifiers: replace,
		},
		"partition": schema.StringAttribute{
			Optional:            true,
			Computed:            true,
			Default:             stringdefault.StaticString("aws"),
			MarkdownDescription: `The AWS partition (aws or aws-us-gov). Defaults to aws if not specified.`,
			Validators: []validator.String{
				stringvalidator.RegexMatches(installaws.AwsPartitionRegex, "AWS partition must be one of: aws, aws-us-gov."),
			},
			PlanModifiers: replace,
		},
		"pool_size": schema.Int64Attribute{
			Optional:            true,
			Computed:            true,
			Default:             int64default.StaticInt64(defaultPoolSize),
			MarkdownDescription: `The number of IAM roles that P0 puts resource grants on. Requests share these roles, so the pool limits how many resource grants can be active at the same time. Defaults to 10.`,
			Validators:          []validator.Int64{int64validator.AtLeast(1)},
			PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
		},
		"identity_provider": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: `The identity provider of the federation app: 'okta' or 'pingone'`,
			Validators:          []validator.String{stringvalidator.OneOf("okta", "pingone")},
			PlanModifiers:       replace,
		},
		"app": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: `The ID of the installed federation app, in P0's Okta (AWS IAM Federation) or PingOne (AWS IAM Federation) integration, that the AWS account trusts`,
			PlanModifiers:       replace,
		},
	}
}

func (m *federationConfigModel) toJson() *federationJson {
	partition := m.Partition.ValueString()
	poolSize := strconv.FormatInt(m.PoolSize.ValueInt64(), 10)
	return &federationJson{
		AccountId:    m.AccountId.ValueStringPointer(),
		AwsPartition: &installaws.AwsPartition{Type: &partition},
		PoolSize:     &poolSize,
		Provider:     &providerJson{Type: m.Provider.ValueString(), App: m.App.ValueString()},
	}
}

func configFromJson(id string, json *federationJson) federationConfigModel {
	model := federationConfigModel{
		Id:        types.StringValue(id),
		AccountId: types.StringPointerValue(json.AccountId),
		Partition: types.StringValue("aws"),
		PoolSize:  types.Int64Value(defaultPoolSize),
		Provider:  types.StringNull(),
		App:       types.StringNull(),
	}
	if json.AwsPartition != nil && json.AwsPartition.Type != nil {
		model.Partition = types.StringValue(*json.AwsPartition.Type)
	}
	if json.PoolSize != nil {
		if size, err := strconv.ParseInt(*json.PoolSize, 10, 64); err == nil {
			model.PoolSize = types.Int64Value(size)
		}
	}
	if json.Provider != nil {
		model.Provider = types.StringValue(json.Provider.Type)
		model.App = types.StringValue(json.Provider.App)
	}
	return model
}
