package common

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

func ReportConversionError(header string, subheader string, value any, diags *diag.Diagnostics) {
	marshalled, marshallErr := json.MarshalIndent(value, "", "  ")
	if marshallErr != nil {
		marshalled = []byte("<An unparseable entity>")
	}
	diags.AddError(header, fmt.Sprintf("%s:\n%s", subheader, marshalled))
}
