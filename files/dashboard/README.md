# PTD Dashboard Pilot Kit

This folder contains a small, read-only pilot kit for building dashboards over the restored SAP ECC SQL Server database `PTD_READONLY`.

The intent is not to expose raw SAP tables directly to business users. The intent is to start from a short list of validated, dashboard-friendly SQL surfaces.

## Files

- `ptd_dashboard_probe.sh`
  - Runs a bounded smoke test over `ssh msi-1` using `sqlcmd`.
  - Writes pipe-delimited output files to `files/dashboard/output`.
- `ptd_sql_tunnel.sh`
  - Opens a local SSH tunnel to the Windows SQL Server host.
  - Useful for browser BI tools running on the Mac.
- `ptd_dashboard_starter_queries.sql`
  - Starter query pack for quick exploration in SSMS, `sqlcmd`, Power BI native SQL, or Metabase native SQL.
- `ptd_dashboard_dataset_blueprints.sql`
  - Stable dataset queries designed for dashboard authorship.
  - Use these as native queries first, then promote stable ones into views on a writable analytics clone later.
- `catalog/*.sql`
  - One-file-per-dataset SQL catalog for a future read-only dataset API.
- `contracts/*.json`
  - JSON metadata contracts that define dataset IDs, columns, planned filters, and row limits.
- `../apps/ptd-api`
  - Minimal Go API that loads the contracts, serves dataset metadata, and can execute contract-backed SQL when `PTD_SQLSERVER_DSN` is configured.
- `../apps/ptd-web`
  - Minimal React dataset explorer that reads the API, exposes supported filters, and previews generated SQL with `dry_run=true`.
- `metabase/docker-compose.yml`
  - Local OSS browser dashboard pilot using Metabase.
- `metabase/README.md`
  - Step-by-step guide for running Metabase against the tunneled SQL Server connection.

## Validated assumptions

- Database: `PTD_READONLY`
- Access mode: `READ_ONLY`
- Scope: client `200`
- Transport/runtime rule:
  - use transparent-table joins already validated in the project notes
  - avoid treating cluster-table semantics as if they were normal direct SQL facts

## Run the probe

From repository root:

```bash
OUT_DIR=files/dashboard/output ./files/dashboard/ptd_dashboard_probe.sh msi-1
```

The probe currently checks:

- database identity
- FI document origin by month
- billing-origin FI slice
- open AR mix
- cleared AR by month
- purchasing history
- inventory movement
- current stock

## Metabase pilot

If you want the fastest OSS browser test:

```bash
./files/dashboard/ptd_sql_tunnel.sh start msi-1 11433
docker compose -f files/dashboard/metabase/docker-compose.yml up -d
```

Then open:

```text
http://localhost:3000
```

And in the Metabase SQL Server connection form use:

- host: `host.docker.internal`
- port: `11433`
- database: `PTD_READONLY`
- schema: `ptd`

## Use in Power BI Desktop

Recommended first mode:

- connect to local SQL Server
- start with `Import`, not `DirectQuery`
- use one dataset blueprint per dashboard page

Good first pages:

1. FI origin by month and company code
2. AR open vs cleared lifecycle
3. purchasing history by `VGABE`
4. current stock by plant and storage location

## Use in Metabase

Recommended first mode:

- create one native query per subject area
- expose filters for month, company code, plant, currency, and document type
- convert only the most stable queries into saved questions or models

## Next upgrade

If the pilot is accepted, move stable queries into one of these:

- SQL views on a writable analytics copy
- governed datasets inside the BI tool
- a small read-only API if a custom frontend is needed later

The contract seed for that API now lives here:

- `files/dashboard/catalog`
- `files/dashboard/contracts`
