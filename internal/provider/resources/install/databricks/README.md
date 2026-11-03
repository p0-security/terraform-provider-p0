# Databricks Resources

## Working on the Databricks Resources

The Databricks resources live in this package. They use `internal/common` as it is. Where they need
a helper that `internal/common` doesn't offer, such as the install `ModifyPlan` in `install.go`, the
package keeps its own copy rather than changing shared code, so a Databricks change doesn't reach
other integrations' resources.

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
