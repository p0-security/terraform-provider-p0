resource "p0_kubernetes_staged" "test" {
  id                    = "tf-staged-test-cluster"
  connectivity_type     = "proxy"
  hosting_type          = "aws"
  cluster_arn           = "arn:aws:eks:us-west-2:123456789101:cluster/my-eks-cluster"
  cluster_endpoint      = "https://asdfgdfsafasdafsafadfasdfasd.gr7.us-west-2.eks.amazonaws.com"
  certificate_authority = "ABIGBASE64CERTSTRING"
}