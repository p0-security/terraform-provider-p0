package installaws

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestCommercialRegionValidator(t *testing.T) {
	cases := map[string]bool{
		"us-east-1":      true,
		"us-west-2":      true,
		"eu-central-2":   true,
		"ap-southeast-7": true,
		"ca-west-1":      true,
		"mx-central-1":   true,
		"il-central-1":   true,
		"us-gov-west-1":  false,
		"us-gov-east-1":  false,
		"cn-north-1":     false,
		"us-isob-east-1": false,
		"eusc-de-east-1": false,
		"us-west":        false,
		" us-east-1":     false,
		"":               false,
	}

	for region, want := range cases {
		resp := &validator.StringResponse{}
		CommercialRegionValidator("commercial regions only").ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("region"),
			ConfigValue: types.StringValue(region),
		}, resp)
		if got := !resp.Diagnostics.HasError(); got != want {
			t.Errorf("CommercialRegionValidator accepts %q = %v, want %v", region, got, want)
		}
	}
}
