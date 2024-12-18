## Local testing

To test with the P0 Terraform provider, add the following to your `.terraformrc`:

```hcl
provider_installation {

  dev_overrides {
      "hashicorp.com/p0-security/p0" = "/path/to/godir/go/bin"
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
      source = "hashicorp.com/p0-security/p0"
    }
  }
}

provider "p0" {
  org = "p0-nathan"
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
  org = "p0-nathan"
  host = "http://localhost:8088/"
}
```
