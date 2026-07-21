# Completes a MySQL (AWS RDS) installation. Set `id` to the same identifier used
# when staging the install. See the p0_mysql_staged example for the full chain:
# p0_aws_rds -> p0_mysql_staged -> P0 Lambda connector -> p0_mysql.
resource "p0_mysql" "example" {
  id         = "my-mysql-instance"
  port       = "3306"
  default_db = "myapp"
}
