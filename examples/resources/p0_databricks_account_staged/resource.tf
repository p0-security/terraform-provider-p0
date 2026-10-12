# Requires the connector that reaches the account to be installed (see
# p0_databricks_connector). The connector's id is the AWS account it runs in.

resource "p0_databricks_account_staged" "example" {
  id           = "01234567-89ab-cdef-0123-456789abcdef"
  connector    = "123456789012"
  accounts_url = "https://accounts.cloud.databricks.com"
}

# See the p0_databricks_account example for the next steps (creating the
# service principal that the connector federates into) that complete the
# install.
