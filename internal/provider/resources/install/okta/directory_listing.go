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
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &OktaDirectoryListing{}
var _ resource.ResourceWithImportState = &OktaDirectoryListing{}
var _ resource.ResourceWithConfigure = &OktaDirectoryListing{}

func NewOktaDirectoryListing() resource.Resource {
	return &OktaDirectoryListing{}
}

type OktaDirectoryListing struct {
	installer *common.Install
}

type oktaDirectoryListingModel struct {
	Client basetypes.StringValue `tfsdk:"client"`
	Domain string                `tfsdk:"domain"`
	Jwk    types.Object          `tfsdk:"jwk"`
}

type oktaDirectoryListingJson struct {
	KeyId     string  `json:"keyId"`
	PublicKey *string `json:"publicKey"`
	State     string  `json:"state"`
	ClientId  *string `json:"clientId"`
}

type oktaDirectoryListingApi struct {
	Item oktaDirectoryListingJson `json:"item"`
}

func (r *OktaDirectoryListing) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_okta_directory_listing"
}

func (r *OktaDirectoryListing) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Final installation of P0 for Okta directory listing.

To use this resource, you must also:
- install the ` + "`p0_okta_directory_listing_staged`" + ` resource,
- add the JWK from that resource to the Okta organization,

See the example usage for the recommended pattern to define this infrastructure.`,
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The domain of the Okta organization",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"client": schema.StringAttribute{
				Required: true,
				MarkdownDescription: `The client ID for the Okta application, shown in the Okta admin console "Applications" page
or as the ` + "`client_id`" + ` attribute of the ` + "`okta_app_oauth`" + ` resource`,
			},
			"jwk": schema.ObjectAttribute{
				Required:            true,
				MarkdownDescription: "The JSON Web Key (JWK) for the Okta application",
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

func (r *OktaDirectoryListing) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  "okta",
		Component:    installresources.DirectoryListing,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *OktaDirectoryListing) getId(data any) *string {
	model, ok := data.(*oktaDirectoryListingModel)
	if !ok {
		return nil
	}
	return &model.Domain
}

func (r *OktaDirectoryListing) getItemJson(json any) any {
	api, ok := json.(*oktaDirectoryListingApi)
	if !ok {
		return nil
	}
	return api.Item
}

func (r *OktaDirectoryListing) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, jsonData any) any {
	data := oktaDirectoryListingModel{}
	api, ok := jsonData.(oktaDirectoryListingJson)
	if !ok {
		return nil
	}

	var jwk Jwk
	if err := json.Unmarshal([]byte(*api.PublicKey), &jwk); err != nil {
		diags.AddError("Error parsing JWK", err.Error())
		return nil
	}
	jwkObj := GetJwkObject(ctx, diags, jwk)
	if jwkObj == nil {
		return nil
	}
	data.Jwk = *jwkObj
	data.Domain = id
	data.Client = basetypes.NewStringPointerValue(api.ClientId)
	return &data
}

func (r *OktaDirectoryListing) toJson(data any) any {
	datav, ok := data.(*oktaDirectoryListingModel)
	if !ok {
		return nil
	}
	out := oktaDirectoryListingApi{}
	jwk := Jwk{}
	// Unpack datav.Jwk into jwk...
	if err := datav.Jwk.As(context.Background(), &jwk, basetypes.ObjectAsOptions{}); err != nil {
		return nil
	}
	jwkBytes, _ := json.Marshal(jwk)
	jwkString := string(jwkBytes)
	out.Item.PublicKey = &jwkString
	out.Item.KeyId = jwk.Kid
	out.Item.State = "configure"
	out.Item.ClientId = datav.Client.ValueStringPointer()
	return &out.Item
}

func (r *OktaDirectoryListing) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json oktaDirectoryListingApi
	var data oktaDirectoryListingModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *OktaDirectoryListing) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json oktaDirectoryListingApi
	var data oktaDirectoryListingModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *OktaDirectoryListing) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json oktaDirectoryListingApi
	var data oktaDirectoryListingModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *OktaDirectoryListing) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data oktaDirectoryListingModel
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &data)
}

func (r *OktaDirectoryListing) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("domain"), req, resp)
}
