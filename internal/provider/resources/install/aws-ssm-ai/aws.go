package installawsssmai

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
)

// A copy of the Redshift (AI) install resources' aws.go, without the cluster
// key, and with the app's messages for a region P0 can't reach.

// An AWS region, such as us-west-2.
var awsRegionRegex = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+-\d+$`)

// A region outside AWS's commercial partition: GovCloud, China, the ISO regions
// or the European Sovereign Cloud. It lists the other partitions rather than the
// commercial regions, so a new commercial region needs no change here.
var awsOtherPartitionRegionRegex = regexp.MustCompile(`^(?:us-gov-|cn-|[a-z]+-iso[a-z]*-|eusc-)`)

// What to enter in place of a region P0 can't reach, in the words of the app's
// install hooks (packages/integrations/aws-ssm-ai/backend/src/install/index.ts and
// shared/src/components.ts).
const (
	invalidRegion          = "Choose a commercial AWS region, such as us-west-2. P0 can't reach GovCloud or China."
	invalidSecretsRegion   = "Enter a commercial AWS region for the secrets, such as us-west-2. P0 can't reach GovCloud or China."
	invalidInstanceRegions = "Enter commercial AWS regions, such as [\"us-west-2\", \"us-east-1\"]. P0 can't reach GovCloud or China."
)

// Rejects a value that isn't an AWS account ID, with the message of the AWS
// install resources.
func awsAccountIdValidator() validator.String {
	return stringvalidator.RegexMatches(installaws.AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits")
}

// Rejects a value that isn't an AWS region, or is a region outside AWS's
// commercial partition, and says what to enter with hint. P0 invokes the
// connector, and builds the secrets' ARNs, in the `aws` partition, so neither
// can be anywhere else.
func commercialRegionValidator(hint string) validator.String {
	return commercialRegion{hint: hint}
}

type commercialRegion struct {
	hint string
}

func (v commercialRegion) Description(context.Context) string {
	return v.hint
}

func (v commercialRegion) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v commercialRegion) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	region := req.ConfigValue.ValueString()
	// Each message quotes the value, so that whitespace around it shows.
	detail := fmt.Sprintf("%q isn't a commercial AWS region. %s", region, v.hint)
	switch {
	case !awsRegionRegex.MatchString(region):
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid AWS region", detail)
	case awsOtherPartitionRegionRegex.MatchString(region):
		resp.Diagnostics.AddAttributeError(req.Path, "Unsupported AWS region", detail)
	}
}
