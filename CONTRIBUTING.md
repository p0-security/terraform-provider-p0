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

## Working on the Databricks Resources

The Databricks resources live in `internal/provider/resources/install/databricks`. They use
`internal/common` as it is. Where they need a helper that `internal/common` doesn't offer, such as
the install `ModifyPlan` in `install.go`, the package keeps its own copy rather than changing shared
code, so a Databricks change doesn't reach other integrations' resources.

Their tests are in `internal/provider`, so CI runs them. `TestDatabricks` and the other unit tests
install each resource against a fake P0 API (`databricks_fake_test.go`), and need neither `TF_ACC`
nor a P0 organization:

```bash
go test ./internal/provider -run Databricks
```

The fake mirrors each component's install schema in the app's
`packages/integrations/databricks/shared/src/components.ts`. When the app changes a component's
fields, update `databricksSchema` in `databricks_fake_test.go` to match.

After you change an example in `examples/resources/p0_databricks_*` or a resource's schema, run
`go generate ./...` and commit the regenerated `docs/`. CI fails if they differ. The
`connector_image_tag` in the `p0_databricks_connector` example changes whenever the app bumps
`CONNECTOR_IMAGE`; the app's `packages/integrations/databricks/README.md` describes that bump.

## Running the Databricks Acceptance Tests

`TestAccDatabricks` installs the Databricks resources into a real P0 organization. It skips unless
`TF_ACC`, `P0_ORG`, `P0_API_TOKEN` and every `P0_DATABRICKS_*` variable that
`internal/provider/databricks_test.go` lists are set, because P0 verifies each install against a
real connector, account, workspace and catalog. `P0_HOST` is optional, and defaults to
`https://api.p0.app`. Use an organization set aside for tests, because the test creates and deletes
real installs.

## Debugging the P0 Terraform Provider with VS Code

Create a file `.env` in the home directory and add your api token and any other environment variables terraform should consume. Environment variables from the shell where `terraform plan/apply` is run will not be used when the debugger is in use.

```bash
P0_API_TOKEN=...
```

Start a new debugging session using the `Debug Terraform Plugin` configuration. Make sure you have the go extension installed in VSCode, then you can simply run the debugger via VS Code. You may need to install delve (go debugger) if it prompts you.

Running this configuration will output an environment variable `TF_REATTACH_PROVIDERS` to your `DEBUG_CONSOLE` which must be used to attach the debugger to the running `terraform apply` process. These variables are set in the shell where the `terraform apply` process is run, not the .env file. Afterwards you can set breakpoints in the provider code and they will be hit when the terraform process is executed.

Example:

```bash
TF_REATTACH_PROVIDERS='{"registry.terraform.io/p0-security/p0":{"Protocol":"grpc","ProtocolVersion":6,"Pid":63519,"Test":true,"Addr":{"Network":"unix","String":"path-to-socket"}}}' terraform apply
```
