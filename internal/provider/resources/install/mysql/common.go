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
