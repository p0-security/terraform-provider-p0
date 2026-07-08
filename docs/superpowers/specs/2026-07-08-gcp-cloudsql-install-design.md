# Design: Terraform provider resources for `p0_gcp_cloudsql`

Linear: [VEL-96](https://linear.app/p0-security/issue/VEL-96/terraform-provider-for-p0-cloudsql)
Design doc: [GCP Cloud Run CloudSQL Design](https://app.notion.com/p/GCP-Cloud-Run-CloudSQL-Design-36c5dc46d35180409911e248b1da6afb)

## Goal

Add Terraform provider resources that let users **stage** and **install** the P0
`gcp-cloudsql` integration, so that P0 can manage just-in-time IAM access to GCP
CloudSQL (PostgreSQL / MySQL) database instances.

## Background

In the P0 app backend, `gcp-cloudsql` is a **standalone VPC-level integration**
(`backend/src/integrations/resources/gcp-cloudsql`, shared schema in
`shared/src/integrations/resources/gcp-cloudsql`):

- **key:** `gcp-cloudsql`
- **itemKind:** `VPC` — the item id is a GCP VPC (network) identifier
- **installable component:** `iam-write` (the only component)
- Unlike AWS, there is no separate DB-level integration; `gcp-cloudsql` directly
  serves both `postgres` and `mysql` CloudSQL engines (selected per access
  request via an `engine` parameter). This resource only concerns **install**,
  not access.

### `iam-write` config schema (from `components.ts`)

| Field | Backend element type | Meaning |
|---|---|---|
| `projectId` | `dynamic`, step `new` | GCP project the VPC lives in. User-supplied at stage time. |
| `connectorRegion` | `hidden`, step `new`, default `us-west1` | Cloud Run region. Backend applies default. |
| `connectorServiceName` | `generated` | `p0-db-{vpcId}` (assigned by the assembler). |
| `connectorServiceUri` | `generated` | Invocation URL of the connector's Cloud Run service; resolved once at `verify` time. |
| `connectorServiceAccount` | `generated` | `p0-db-{vpcId}@{projectId}.iam.gserviceaccount.com`. |

### Install lifecycle (backend)

- **assemble** (triggered by staging / `PUT`): computes `connectorServiceName`,
  `connectorServiceAccount`, `connectorRegion` from `vpcId` + `projectId`. These
  are deterministic, so they are known immediately after staging.
- **verify** (triggered by the `verify` install step): looks up the deployed
  Cloud Run connector service, confirms P0's service account may invoke it, and
  persists its `connectorServiceUri`. Fails with a retryable `StateError` if the
  connector is not yet deployed / reachable.

The consequence: a deployed Cloud Run connector **must exist between staging and
verification**. This is why we mirror the `p0_mysql_staged` → `p0_mysql` two-step
pattern rather than the single-shot `p0_aws_rds` pattern.

## Approach

Two Terraform resources in a new package
`internal/provider/resources/install/gcp-cloudsql` (package `installgcpcloudsql`):

1. `p0_gcp_cloudsql_staged` — stages the integration and exposes the generated
   connector identifiers so the user can deploy the Cloud Run connector (via the
   `p0-connector/gcp` module).
2. `p0_gcp_cloudsql` — completes the install by running `verify` + `configure`,
   which resolves `connector_service_uri` and advances the item to `installed`.

Both build on the shared `common.Install` helper, following the existing
`postgres` / `mysql` / `rds` resources.

### Files

```
internal/provider/resources/install/gcp-cloudsql/
  common.go             # GcpCloudSqlKey, Components, validation regexes
  iam_write_staged.go   # p0_gcp_cloudsql_staged
  iam_write.go          # p0_gcp_cloudsql
```

### `common.go`

- `const GcpCloudSqlKey = "gcp-cloudsql"`
- `var Components = []string{installresources.IamWrite}`
- `GcpProjectIdRegex` — GCP project ID format (`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`),
  used to validate `project_id`.
- **No** format validation on `id` (VPC identifier) — accept any non-empty
  string and let the backend reject invalid values. (Decision: keep `id` lenient.)

### Shared model / JSON contract

API item shape (`{ "item": { ... } }`):

```go
type gcpCloudSqlIamWriteJson struct {
    ProjectId               string  `json:"projectId"`
    ConnectorRegion         *string `json:"connectorRegion,omitempty"`
    ConnectorServiceName    *string `json:"connectorServiceName,omitempty"`
    ConnectorServiceUri     *string `json:"connectorServiceUri,omitempty"`
    ConnectorServiceAccount *string `json:"connectorServiceAccount,omitempty"`
    State                   string  `json:"state"`
}
```

- `toJson` sends **only** `projectId` (region omitted so the backend applies its
  `us-west1` default; generated fields are backend-owned).
- `fromJson` maps every field back into the model. Pointer/optional JSON fields
  become `types.StringNull()` when absent.

### Attribute schema (both resources, identical)

| Attribute | Mode | Notes |
|---|---|---|
| `id` | Required, `RequiresReplace` | GCP VPC identifier. No format validator. |
| `project_id` | Required, `RequiresReplace` | Validated by `GcpProjectIdRegex`. Maps to `projectId`. |
| `region` | Computed | Backend-assigned Cloud Run region (`us-west1`). Not user-settable. |
| `connector_service_name` | Computed | `p0-db-{id}`. |
| `connector_service_account` | Computed | Connector's GCP service account email. |
| `connector_service_uri` | Computed | Connector Cloud Run URL; null until installed. |
| `state` | Computed | `common.StateMarkdownDescription`. |

`region` is `Computed` only (per decision to treat the backend `hidden` region as
non-user-facing). It is populated from the backend response.

### `p0_gcp_cloudsql_staged` behavior

- `Metadata` → `p0_gcp_cloudsql_staged`
- `Configure` → `common.Install{ Integration: GcpCloudSqlKey, Component: IamWrite, ... }`
- `Create` → `EnsureConfig` + `Stage(inputJson=toJson(plan))`
- `Update` → `EnsureConfig` + `Stage` (re-stage)
- `Read`   → `Install.Read`
- `Delete` → `Install.Delete`
- `ImportState` → passthrough `id`

### `p0_gcp_cloudsql` behavior

- `Metadata` → `p0_gcp_cloudsql`
- `Configure` → same installer config
- `Create` → `EnsureConfig` + `Stage` + `UpsertFromStage` (runs `verify` then
  `configure`; resolves `connector_service_uri`)
- `Update` → `UpsertFromStage`
- `Read`   → `Install.Read`
- `Delete` → `Install.Rollback` (returns item to `stage` rather than deleting, so
  a co-located `_staged` resource's delete does not double-delete — matches
  `postgres`)
- `ImportState` → passthrough `id`

### Provider wiring

Register both constructors in `internal/provider/provider.go` `Resources` list:

```go
installgcpcloudsql.NewGcpCloudSqlIamWriteStaged,
installgcpcloudsql.NewGcpCloudSqlIamWrite,
```

### Examples & docs

- `examples/resources/p0_gcp_cloudsql_staged/resource.tf`
- `examples/resources/p0_gcp_cloudsql/resource.tf`

Example narrative: create `p0_gcp_cloudsql_staged` → use its
`connector_service_name` / `connector_service_account` outputs to deploy the
Cloud Run connector (`p0-connector/gcp` module) → create `p0_gcp_cloudsql` with
`id = p0_gcp_cloudsql_staged.example.id` and `depends_on` the connector.

- Regenerate docs (`docs/resources/gcp_cloudsql_staged.md`,
  `docs/resources/gcp_cloudsql.md`) via `tfplugindocs` / the repo's `make docs`
  (or `go generate`).

## Testing

- Follow the existing pattern (the repo uses `terraform-plugin-testing` acceptance
  tests / unit tests where present). Add unit coverage for `toJson` / `fromJson`
  round-tripping and for `project_id` validation, matching the style used by
  sibling install resources.
- `go build ./...` and `go vet ./...` must pass; run `gofmt`.

## Out of scope

- Access lifecycle / request resources (this ticket is install only).
- The `p0-connector/gcp` and `p0-cloudsql-vpc/gcp` Terraform modules themselves.
- MySQL access support (backend currently rejects `engine=mysql` at request time).
