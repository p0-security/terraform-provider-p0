package installjiraai

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
)

// The Jira (AI) `connector` item: one Cloud Run deployment of P0's Jira connector.
// p0_jira_ai_connector_staged and p0_jira_ai_connector manage the same item, so they
// share its model, its JSON and its installers. A Google Cloud-only copy of Harbor's
// connector_item.go.

const invalidConnectorId = "Invalid Jira connector ID"

type connectorModel struct {
	Id      types.String  `tfsdk:"id"`
	Hosting *hostingModel `tfsdk:"hosting"`
	State   types.String  `tfsdk:"state"`
}

// The item, as P0 stores it.
type connectorJson struct {
	Hosting *installapp.ConnectorHostingJson `json:"hosting,omitempty"`
	State   *string                          `json:"state,omitempty"`
}

type connectorApi struct {
	Item *connectorJson `json:"item"`
}

// The resources' attributes. P0 fixes the ID and the hosting when it creates the item
// and refuses a change to either, so each requires replacement.
func connectorAttributes(idDescription string, hostingDescription string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id":      fixedAttribute(idDescription, itemIdValidators()...),
		"hosting": hostingAttribute(hostingDescription),
		"state":   common.StateAttribute,
	}
}

// The installer that stages, verifies and configures the item, and rolls it back.
func newConnectorInstaller(data *internal.P0ProviderData) *common.Install {
	return &common.Install{
		Integration:        JiraAiKey,
		Component:          installresources.Connector,
		ProviderData:       data,
		GetId:              connectorId,
		GetItemJson:        connectorItemJson,
		FromJson:           connectorFromJson(missingServiceAccountOnWrite),
		ToJson:             connectorToJson,
		DescribeCheckError: describeCheckError("connector"),
	}
}

// The installer for reads. It differs only in its message for a connector that P0
// returns without its service account, which tells the reader how to plan meanwhile.
// A connector P0 no longer has is dropped from state, and the next apply stages it
// again.
func newConnectorReader(data *internal.P0ProviderData) *common.Install {
	reader := newConnectorInstaller(data)
	reader.FromJson = connectorFromJson(missingServiceAccountOnRead)
	return reader
}

func connectorId(data any) *string {
	model, ok := data.(*connectorModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func connectorItemJson(json any) any {
	// An answer with no item leaves Item nil, which has to come back as an untyped
	// nil: a nil *connectorJson in an any isn't nil, and would reach FromJson.
	inner, ok := json.(*connectorApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner.Item
}

// What follows the reason in the diagnostic for a connector that P0 returned without
// its service account.
const (
	missingServiceAccountOnWrite = "Contact " + supportEmail + "."
	// An error rather than a warning: a null account in state would make the
	// customer's deployment Terraform plan to replace the service account, which every
	// site's secret grants name.
	missingServiceAccountOnRead = "Contact " + supportEmail + ". Until P0 returns the account, plan and apply other changes with `-refresh=false`, " +
		"or remove the connector from Terraform state with `terraform state rm` and import it again once P0 returns it."
)

func connectorFromJson(missingServiceAccount string) func(context.Context, *diag.Diagnostics, string, any) any {
	return func(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
		jsonv, ok := json.(*connectorJson)
		if !ok {
			return nil
		}

		// hosting is Required, so leaving it nil here would surface as Terraform core's
		// "provider produced inconsistent result after apply" rather than something the
		// reader can act on.
		if jsonv.Hosting == nil {
			diags.AddError(
				"Bad API response",
				fmt.Sprintf("The Jira connector %s carries no hosting configuration. "+
					"Remove it in the P0 app and add it again, or remove it from Terraform state with `terraform state rm`.", id),
			)
			return nil
		}

		// The connector must run as the service account P0 names when it stages the item,
		// so it can't be deployed without one.
		if jsonv.Hosting.ConnectorServiceAccount == nil || *jsonv.Hosting.ConnectorServiceAccount == "" {
			diags.AddError(
				"Bad API response",
				fmt.Sprintf("P0 returned the Jira connector %s without its service account. "+
					"The connector must run as the account that P0 names when it stages the connector, so it can't be deployed without one. %s",
					id, missingServiceAccount),
			)
			return nil
		}

		return &connectorModel{
			Id:      types.StringValue(id),
			Hosting: hostingFromJson(jsonv.Hosting),
			State:   types.StringPointerValue(jsonv.State),
		}
	}
}

// The body of every request: where the connector runs. P0 sets the hosting's other
// fields, so toJson leaves them out.
func connectorToJson(data any) any {
	datav, ok := data.(*connectorModel)
	if !ok {
		return nil
	}

	json := connectorJson{}
	if datav.Hosting != nil {
		json.Hosting = datav.Hosting.toJson()
	}
	return &json
}
