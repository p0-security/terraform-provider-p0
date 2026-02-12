package installmysql

import (
	"regexp"

	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const MysqlKey = "mysql"

// All installable MySQL components.
var Components = []string{installresources.IamWrite}

var AwsRdsArnRegex = regexp.MustCompile(`^arn:aws:rds:[a-z]{2}-[a-z]+-\d{1}:\d{12}:(db|cluster):[a-zA-Z0-9-]+$`)
var AwsVpcIdRegex = regexp.MustCompile(`^vpc-[a-f0-9]+$`)
var HostnameRegex = regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)
var PortRegex = regexp.MustCompile(`^\d{1,5}$`)

const MysqlDefaultPort = "3306"

// parseRdsArn extracts the region and account ID from an RDS ARN.
// ARN format: arn:aws:rds:region:account-id:db:instance-id
// or: arn:aws:rds:region:account-id:cluster:cluster-id
// Returns empty strings if the ARN format is invalid.
func parseRdsArn(arn string) (region string, accountId string) {
	arnRegex := regexp.MustCompile(`^arn:aws:rds:([^:]+):([^:]+):([^:]+):(.+)$`)
	matches := arnRegex.FindStringSubmatch(arn)
	if len(matches) != 5 {
		return "", ""
	}
	return matches[1], matches[2]
}
