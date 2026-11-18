package installgcpcloudlogging

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// A project item's key is its project ID, which is safe in a URL path.

type projectStagedModel struct {
	ProjectId types.String `tfsdk:"project_id"`
	connectorModel
}

type projectModel struct {
	projectStagedModel
	ConnectorServiceUri types.String `tfsdk:"connector_service_uri"`
}

func projectGetId(data any) *string {
	var id string
	switch model := data.(type) {
	case *projectStagedModel:
		id = model.ProjectId.ValueString()
	case *projectModel:
		id = model.ProjectId.ValueString()
	default:
		return nil
	}
	return &id
}

func newProjectStagedModel(id string, item *sourceJson) projectStagedModel {
	return projectStagedModel{ProjectId: types.StringValue(id), connectorModel: newConnectorModel(item)}
}

func projectStagedFromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*sourceJson)
	if !ok {
		return nil
	}
	model := newProjectStagedModel(id, item)
	return &model
}

func projectStagedToJson(data any) any {
	model, ok := data.(*projectStagedModel)
	if !ok {
		return nil
	}
	return model.toJson()
}

func projectFromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*sourceJson)
	if !ok {
		return nil
	}
	return &projectModel{
		projectStagedModel:  newProjectStagedModel(id, item),
		ConnectorServiceUri: types.StringPointerValue(item.ConnectorServiceUri),
	}
}

// P0 finds the connector's URL itself, so it is never sent.
func projectToJson(data any) any {
	model, ok := data.(*projectModel)
	if !ok {
		return nil
	}
	return model.toJson()
}

// The organization item is P0's singleton, `_`.

type organizationStagedModel struct {
	connectorModel
}

type organizationModel struct {
	connectorModel
	ConnectorServiceUri types.String `tfsdk:"connector_service_uri"`
}

func singletonGetId(any) *string {
	id := common.SingletonKey
	return &id
}

func organizationStagedFromJson(_ context.Context, _ *diag.Diagnostics, _ string, json any) any {
	item, ok := json.(*sourceJson)
	if !ok {
		return nil
	}
	return &organizationStagedModel{connectorModel: newConnectorModel(item)}
}

func organizationStagedToJson(data any) any {
	model, ok := data.(*organizationStagedModel)
	if !ok {
		return nil
	}
	return model.toJson()
}

func organizationFromJson(_ context.Context, _ *diag.Diagnostics, _ string, json any) any {
	item, ok := json.(*sourceJson)
	if !ok {
		return nil
	}
	return &organizationModel{
		connectorModel:      newConnectorModel(item),
		ConnectorServiceUri: types.StringPointerValue(item.ConnectorServiceUri),
	}
}

func organizationToJson(data any) any {
	model, ok := data.(*organizationModel)
	if !ok {
		return nil
	}
	return model.toJson()
}
