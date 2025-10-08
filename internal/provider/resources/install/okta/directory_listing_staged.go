package installokta

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &OktaDirectoryListingStaged{}
var _ resource.ResourceWithImportState = &OktaDirectoryListingStaged{}
var _ resource.ResourceWithConfigure = &OktaDirectoryListingStaged{}

func NewOktaDirectoryListingStaged() resource.Resource {
	return &OktaDirectoryListingStaged{}
}

type OktaDirectoryListingStaged struct {
	installer *common.Install
}

type oktaDirectoryListingStagedModel struct {
	Domain string       `tfsdk:"domain"`
	Jwk    types.Object `tfsdk:"jwk"`
}

type oktaDirectoryListingStagedJson struct {
	KeyId     string `json:"keyId"`
	PublicKey string `json:"publicKey"`
	State     string `json:"state"`
}

type oktaDirectoryListingStagedApi struct {
	Item oktaDirectoryListingStagedJson `json:"item"`
}

func (r *OktaDirectoryListingStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_okta_directory_listing_staged"
}

func (r *OktaDirectoryListingStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged installation of P0, on an Okta organization for directory listing.

For instructions on using this resource, see the documentation for ` + "`p0_okta_directory_listing`.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The domain of the Okta organization",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"jwk": schema.ObjectAttribute{
				Computed:            true,
				MarkdownDescription: "The public JSON Web Key (JWK) added to the Okta App",
				AttributeTypes: map[string]attr.Type{
					"kty": types.StringType,
					"kid": types.StringType,
					"e":   types.StringType,
					"n":   types.StringType,
				},
			},
		},
	}
}

func (r *OktaDirectoryListingStaged) getId(data any) *string {
	model, ok := data.(*oktaDirectoryListingStagedModel)
	if !ok {
		return nil
	}
	return &model.Domain
}

func (r *OktaDirectoryListingStaged) getItemJson(json any) any {
	return json
}

func (r *OktaDirectoryListingStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, jsonData any) any {
	data := oktaDirectoryListingStagedModel{}
	jsonv, ok := jsonData.(*oktaDirectoryListingStagedApi)
	if !ok {
		return nil
	}

	data.Domain = id

	var jwk Jwk
	if err := json.Unmarshal([]byte(jsonv.Item.PublicKey), &jwk); err != nil {
		diags.AddError("Error parsing JWK", err.Error())
		return nil
	}

	jwkObj := GetJwkObject(ctx, diags, jwk)
	if jwkObj == nil {
		return nil
	}
	data.Jwk = *jwkObj

	return &data
}

func (r *OktaDirectoryListingStaged) toJson(data any) any {
	json := oktaDirectoryListingStagedApi{}
	return &json.Item
}

func (r *OktaDirectoryListingStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  OktaKey,
		Component:    installresources.DirectoryListing,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (s *OktaDirectoryListingStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json oktaDirectoryListingStagedApi
	var data oktaDirectoryListingStagedModel

	s.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &struct{}{})
}

func (s *OktaDirectoryListingStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &oktaDirectoryListingStagedApi{}, &oktaDirectoryListingStagedModel{})
}

func (s *OktaDirectoryListingStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &oktaDirectoryListingStagedModel{})
}

func (s *OktaDirectoryListingStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &oktaDirectoryListingStagedApi{}, &oktaDirectoryListingStagedModel{})
}

func (s *OktaDirectoryListingStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("domain"), req, resp)
}
