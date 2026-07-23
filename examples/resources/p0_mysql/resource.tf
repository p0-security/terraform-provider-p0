# Completes a MySQL (AWS RDS) install; `id` must match the p0_mysql_staged id.
# See the p0_mysql_staged example for the full install chain.
resource "p0_mysql" "example" {
  id         = "my-mysql-instance"
  port       = "3306"
  default_db = "myapp"
}
