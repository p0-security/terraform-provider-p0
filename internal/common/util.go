package common

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

func reportConversionError(header string, subheader string, value any, diags *diag.Diagnostics) {
	marshalled, marshallErr := json.MarshalIndent(value, "", "  ")
	if marshallErr != nil {
		marshalled = []byte("<An unparseable entity>")
	}
	diags.AddError(header, fmt.Sprintf("%s:\n%s", subheader, marshalled))
}

var UuidRegex = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
