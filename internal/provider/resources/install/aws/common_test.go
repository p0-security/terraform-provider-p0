package installaws

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A commercial region passes. A value that isn't a region is told so, and only a region
// in another partition gets the caller's message.
func TestCommercialRegionValidator(t *testing.T) {
	const (
		message     = "commercial regions only"
		invalid     = "Invalid AWS region: Enter an AWS region, such as us-west-2"
		unsupported = "Unsupported AWS region: " + message
	)
	cases := map[string]string{
		"us-east-1":      "",
		"us-west-2":      "",
		"eu-central-2":   "",
		"ap-southeast-7": "",
		"ca-west-1":      "",
		"mx-central-1":   "",
		"il-central-1":   "",
		"us-gov-west-1":  unsupported,
		"us-gov-east-1":  unsupported,
		"cn-north-1":     unsupported,
		"us-isob-east-1": unsupported,
		"eusc-de-east-1": unsupported,
		"us-west":        invalid,
		" us-east-1":     invalid,
		"":               invalid,
	}

	for region, want := range cases {
		resp := &validator.StringResponse{}
		CommercialRegionValidator(message).ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("region"),
			ConfigValue: types.StringValue(region),
		}, resp)

		var got []string
		for _, err := range resp.Diagnostics.Errors() {
			got = append(got, err.Summary()+": "+err.Detail())
		}
		switch {
		case want == "" && len(got) > 0:
			t.Errorf("CommercialRegionValidator rejects %q: %v", region, got)
		case want != "" && (len(got) != 1 || !strings.HasPrefix(got[0], want)):
			t.Errorf("CommercialRegionValidator(%q) = %v, want one error starting %q", region, got, want)
		}
	}
}
