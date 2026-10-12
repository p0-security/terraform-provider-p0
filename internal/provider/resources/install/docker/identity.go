package installdocker

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The identity a connector runs as, which each organization lets read its access token:
// an IAM role on AWS, or a service account on Google Cloud, named after the connector
// in the connector's own account or project. P0 doesn't look it up. It names it
// (`toConnectorIdentity` in packages/integrations/docker/shared/src/identity.ts in the
// app), and returns it as the item's metadata, so the provider needn't derive it.
//
// The JSON is a discriminated union on "type": the AWS variant carries the account and
// the role, the Google Cloud variant the project and the service account.
type connectorIdentityJson struct {
	Type                string  `json:"type"`
	AccountId           *string `json:"accountId,omitempty"`
	RoleArn             *string `json:"roleArn,omitempty"`
	RoleName            *string `json:"roleName,omitempty"`
	ProjectId           *string `json:"projectId,omitempty"`
	ServiceAccountEmail *string `json:"serviceAccountEmail,omitempty"`
	ServiceAccountId    *string `json:"serviceAccountId,omitempty"`
}

type connectorIdentityModel struct {
	Type                types.String `tfsdk:"type"`
	AccountId           types.String `tfsdk:"account_id"`
	RoleArn             types.String `tfsdk:"role_arn"`
	RoleName            types.String `tfsdk:"role_name"`
	ProjectId           types.String `tfsdk:"project_id"`
	ServiceAccountEmail types.String `tfsdk:"service_account_email"`
	ServiceAccountId    types.String `tfsdk:"service_account_id"`
}

var connectorIdentityAttrTypes = map[string]attr.Type{
	"type":                  types.StringType,
	"account_id":            types.StringType,
	"role_arn":              types.StringType,
	"role_name":             types.StringType,
	"project_id":            types.StringType,
	"service_account_email": types.StringType,
	"service_account_id":    types.StringType,
}

func connectorIdentityAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Computed: true,
		MarkdownDescription: `The identity the connector must run as: an IAM role on AWS, or a service account on Google Cloud, named after the connector. ` +
			`P0 doesn't look the identity up, so deploy the connector with exactly this one. Each organization that names the connector lets this identity read its access token.`,
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The identity's cloud: either ` + "`aws`" + ` (an IAM role) or ` + "`gcp`" + ` (a service account)`,
			},
			"account_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The ID of the AWS account the role is in. Only populated for ` + "`aws`" + `.`,
			},
			"role_arn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The ARN of the IAM role the connector's Lambda function runs as. Only populated for ` + "`aws`" + `.`,
			},
			"role_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The name of the IAM role the connector's Lambda function runs as. Only populated for ` + "`aws`" + `.`,
			},
			"project_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The ID of the Google Cloud project the service account is in. Only populated for ` + "`gcp`" + `.`,
			},
			"service_account_email": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The email of the service account the connector's Cloud Run service runs as. Only populated for ` + "`gcp`" + `.`,
			},
			"service_account_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The account ID of the service account the connector's Cloud Run service runs as: the part of its email before the ` + "`@`" + `. Only populated for ` + "`gcp`" + `.`,
			},
		},
	}
}

// Reads the identity from an item's metadata. An item with no metadata, which P0 only
// answers with when it can't name the identity, has a null identity.
func connectorIdentityFromJson(ctx context.Context, diags *diag.Diagnostics, json *connectorIdentityJson) types.Object {
	if json == nil {
		return types.ObjectNull(connectorIdentityAttrTypes)
	}

	identity, objDiags := types.ObjectValueFrom(ctx, connectorIdentityAttrTypes, connectorIdentityModel{
		Type:                types.StringValue(json.Type),
		AccountId:           types.StringPointerValue(json.AccountId),
		RoleArn:             types.StringPointerValue(json.RoleArn),
		RoleName:            types.StringPointerValue(json.RoleName),
		ProjectId:           types.StringPointerValue(json.ProjectId),
		ServiceAccountEmail: types.StringPointerValue(json.ServiceAccountEmail),
		ServiceAccountId:    types.StringPointerValue(json.ServiceAccountId),
	})
	diags.Append(objDiags...)
	return identity
}
