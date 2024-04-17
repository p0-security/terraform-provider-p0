// Copyright (c) 2024 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installaws

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &StagedAws{}
var _ resource.ResourceWithImportState = &StagedAws{}
var _ resource.ResourceWithConfigure = &StagedAws{}

func NewStagedAws() resource.Resource {
	return &StagedAws{}
}

type StagedAws struct {
	data *internal.P0ProviderData
}

type stagedAwsBaseJson = struct {
	ServiceAccountId string `json:"serviceAccountId"`
	State            string `json:"state"`
}

type stagedAwsComponentJson = struct {
	Label *string `json:"label"`
	State *string `json:"state"`
}

type stagedAwsJson struct {
	Config struct {
		Base      map[string]stagedAwsBaseJson      `json:"base"`
		IamWrite  map[string]stagedAwsComponentJson `json:"iam-write"`
		Inventory map[string]stagedAwsComponentJson `json:"inventory"`
	} `json:"config"`
}

type stagedAwsModel struct {
	Id               string       `tfsdk:"id"`
	Label            types.String `tfsdk:"label"`
	ServiceAccountId types.String `tfsdk:"service_account_id"`
	Components       []string     `tfsdk:"components"`
}

func (r *StagedAws) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_staged"
}

func (r *StagedAws) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged AWS installation. Staged resources are used to generate AWS trust policies.

**Important** Before using this resource, please read the instructions for the 'aws_iam_write' resource.
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The AWS account ID`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(AwsAccountIdRegex, "AWS account IDs should be numeric"),
				},
			},
			"label": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The AWS account's alias (if available)`,
			},
			"service_account_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The audience ID of the service account to include in this AWS account's P0 role trust policies`,
			},
			"components": schema.SetAttribute{
				ElementType:         types.StringType,
				Required:            true,
				MarkdownDescription: `Components to install (any of "iam-write", "inventory")`,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.OneOf(Components...),
					),
					setvalidator.SizeAtLeast(1),
				},
			},
		},
	}
}

func (r *StagedAws) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	data := internal.Configure(&req, resp)
	if data != nil {
		r.data = data
	}
}

func (r *StagedAws) itemPath(component string, id string) string {
	return fmt.Sprintf("integrations/aws/config/%s/%s", component, id)
}

func (r *StagedAws) fromComponentJson(data *stagedAwsModel, json *stagedAwsComponentJson, component string) {
	if json.Label != nil {
		data.Label = types.StringValue(*json.Label)
	}
	data.Components = append(data.Components, component)
}

func (r *StagedAws) fromJson(data *stagedAwsModel, json *stagedAwsJson) {
	data.Components = []string{}
	data.Label = types.StringNull()
	data.ServiceAccountId = types.StringNull()

	base, okBase := json.Config.Base[data.Id]
	if okBase {
		data.ServiceAccountId = types.StringValue(base.ServiceAccountId)
	}

	iamWriteJson, okIamWrite := json.Config.IamWrite[data.Id]
	if okIamWrite {
		r.fromComponentJson(data, &iamWriteJson, IamWrite)
	}

	inventoryJson, okInventory := json.Config.Inventory[data.Id]
	if okInventory {
		r.fromComponentJson(data, &inventoryJson, Inventory)
	}
}

func (r *StagedAws) readState(ctx context.Context, diags *diag.Diagnostics, data *stagedAwsModel, state *tfsdk.State) {
	var config stagedAwsJson
	httpErr := r.data.Get("integrations/aws/config", &config)
	if httpErr != nil {
		diags.AddError("Error communicationg with P0", fmt.Sprintf("Unable to read AWS configuration, got error:\n%s", httpErr))
		return
	}
	r.fromJson(data, &config)

	// Save updated data into Terraform state
	diags.Append(state.Set(ctx, data)...)
}

func (r *StagedAws) put(ctx context.Context, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State, operation string) {
	var data stagedAwsModel
	diags.Append(plan.Get(ctx, &data)...)
	if diags.HasError() {
		return
	}

	report := func(component string, err error) {
		if err != nil {
			diags.AddError(fmt.Sprintf("Could not %s %s component", operation, component), fmt.Sprintf("Error: %s", err))
		}
	}

	for _, component := range Components {
		path := r.itemPath(component, data.Id)
		if slices.Contains(data.Components, component) {
			var json stagedAwsComponentJson
			// We need to read back the entire configuration anyway (to get the base config), so ignore post responses
			err := r.data.Put(path, &struct{}{}, &json)
			report(component, err)
		} else {
			err := r.data.Delete(path)
			report(component, err)
		}
	}

	if diags.HasError() {
		return
	}

	// Save updated data into Terraform state
	r.readState(ctx, diags, &data, state)
}

func (r *StagedAws) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.put(ctx, &resp.Diagnostics, &req.Plan, &resp.State, "create")
}

func (r *StagedAws) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data stagedAwsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.readState(ctx, &resp.Diagnostics, &data, &resp.State)
}

func (r *StagedAws) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.put(ctx, &resp.Diagnostics, &req.Plan, &resp.State, "update")
}

func (r *StagedAws) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data stagedAwsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, component := range Components {
		path := r.itemPath(component, data.Id)
		err := r.data.Delete(path)
		if err != nil {
			resp.Diagnostics.AddError("Could not delete component", fmt.Sprintf("%s", err))
			return
		}
	}
}

func (r *StagedAws) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
