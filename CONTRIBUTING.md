# Contribution Guide

## Running the P0 Terraform Provider Locally

To test with the P0 Terraform provider, add the following to your `.terraformrc`:

```hcl
provider_installation {

  dev_overrides {
      "p0-security/p0" = "/path/to/godir/go/bin"
  }

  # For all other providers, install them directly from their origin provider
  # registries as normal. If you omit this, Terraform will _only_ use
  # the dev_overrides block, and so no other providers will be available.
  direct {}
}
```

Then, create a `examples/provider-install-verification` directory in this
repository (this path is permanently added to `.gitignore`), and add a `main.tf`:

```hcl
terraform {
  required_providers {
    p0 = {
      source = "p0-security/p0"
    }
  }
}

provider "p0" {
  org = "org-id"
}
```

Now, build this provider:

```bash
go install
```

You can now test locally. In the `examples/provider-install-verification` directory:

```bash
export P0_API_TOKEN=...
terraform plan
```

If you are using a local build of the P0 API server, you can also set that in your
`main.tf`:

```
provider "p0" {
  org = "org-id"
  host = "http://my-custom-host/"
}
```

## Debugging the P0 Terraform Provider with VS Code

Create a file `.env` in the home directory and add your api token and any other environment variables terraform should consume. Environment variables from the shell where `terraform plan/apply` is run will not be used when the debugger is in use.

```bash
P0_API_TOKEN=...
```

Start a new debugging session using the `Debug Terraform Plugin` configuration. Running this configuration will output an environment variable `TF_REATTACH_PROVIDERS` to your `DEBUG_CONSOLE` which must be used to attach the debugger to the running `terraform apply` process. These variables are set in the shell where the `terraform apply` process is run, not the .env file. Afterwards you can set breakpoints in the provider code and they will be hit when the terraform process is executed.

Example:

```bash
TF_REATTACH_PROVIDERS='{"registry.terraform.io/p0-security/p0":{"Protocol":"grpc","ProtocolVersion":6,"Pid":63519,"Test":true,"Addr":{"Network":"unix","String":"path-to-socket"}}}' terraform apply
```
