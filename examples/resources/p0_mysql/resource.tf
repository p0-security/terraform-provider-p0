resource "p0_mysql" "example" {
  id           = p0_mysql_staged.example.id
  instance_arn = p0_mysql_staged.example.instance_arn
  vpc_id       = p0_mysql_staged.example.vpc_id
  port         = "3306"
  default_db   = "myapp"
  depends_on   = [aws_lambda_function.mysql_connector]
}
