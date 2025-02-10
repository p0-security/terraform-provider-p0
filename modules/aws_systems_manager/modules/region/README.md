This module enables default host management for Systems Manager and creates VPC Endpoints in all subnets of the specified VPCs.
The module is scoped to a single AWS region of an AWS account.

The VPCs passed to this module must satisfy the prerequisites for VPC Endpoints:
https://docs.aws.amazon.com/vpc/latest/privatelink/create-interface-endpoint.html#prerequisites-interface-endpoints
