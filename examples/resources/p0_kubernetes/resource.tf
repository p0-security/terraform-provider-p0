resource "p0_kubernetes" "tf-test-cluster" {
  id         = "tf-staged-test-cluster"
  token      = "ABIGBASE64TOKENSTRING"
  public_jwk = "{\"kty\":\"XYZ\",\"x\":\"7KbPMM9PZVnH-WZqFD-K1MH7QLQr5beqSVKdst9AhV5\",\"y\":\"d4RrlJUVPOAfyYBuPtmircWsFfV80VPrWYkTAHV1Qww5-gV\",\"crv\":\"P-404\"}"

  connectivity_type     = "proxy"
  hosting_type          = "aws"
  cluster_arn           = "arn:aws:eks:us-west-2:123456789101:cluster/my-eks-cluster"
  cluster_endpoint      = "https://asdfgdfsafasdafsafadfasdfasd.gr7.us-west-2.eks.amazonaws.com"
  certificate_authority = "ABIGBASE64CERTSTRING"
}