package internal

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func Configure(req *resource.ConfigureRequest, resp *resource.ConfigureResponse) *P0ProviderData {
	if req.ProviderData == nil {
		return nil
	}

	data, ok := req.ProviderData.(P0ProviderData)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected P0ProviderData, got: %T. Please report this issue to support@p0.dev.", req.ProviderData),
		)

		return nil
	}
	return &data
}
