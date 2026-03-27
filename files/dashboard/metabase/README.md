# Metabase Pilot For `PTD_READONLY`

This is the quickest OSS browser-dashboard pilot path for the restored SAP ECC SQL Server copy.

## Goal

Run Metabase locally, then connect it to the MSSQL database without exposing SQL Server broadly on the network.

## Recommended topology on macOS

1. Open an SSH tunnel from the Mac to the Windows host.
2. Run Metabase locally in Docker.
3. In Metabase, add SQL Server as a source using the tunneled port.

## Start the SQL tunnel

From repository root:

```bash
./files/dashboard/ptd_sql_tunnel.sh start msi-1 11433
```

Check status:

```bash
./files/dashboard/ptd_sql_tunnel.sh status msi-1 11433
```

Stop it:

```bash
./files/dashboard/ptd_sql_tunnel.sh stop msi-1 11433
```

The tunnel helper is verified against the live SQL listener on `msi-1`. If you want an API-level smoke test before opening Metabase, run:

```bash
cd apps/ptd-api && \
  PTD_SQLSERVER_DSN="sqlserver://ptd_reader:password@127.0.0.1:11433?database=PTD_READONLY&encrypt=disable" \
  GOTOOLCHAIN=local \
  go run ./cmd/ptd-api --validate
```

## Start Metabase

From repository root:

```bash
docker compose -f files/dashboard/metabase/docker-compose.yml up -d
```

Open:

```text
http://localhost:3000
```

## Add the SQL Server source in Metabase

Use these values in the Metabase UI:

- Database type: `SQL Server`
- Host:
  - `host.docker.internal` when Metabase runs in Docker on macOS and you use the SSH tunnel above
  - `localhost` when Metabase runs directly on the Windows host itself
- Port: `11433` for the SSH tunnel example above
- Database name: `PTD_READONLY`
- Username / password:
  - use a dedicated read-only SQL login if you create one later
  - or use a temporary trusted/testing path only in a non-production pilot
- Schema: `ptd`

## First dashboards to build

Use the native SQL editor in Metabase and paste datasets from:

- `files/dashboard/ptd_dashboard_dataset_blueprints.sql`

Start with:

1. FI document origin monthly
2. AR open item monthly
3. AR cleared item monthly
4. Purchasing history monthly
5. Inventory movement monthly
6. Current stock by plant and storage location

## Why this pilot is useful

- browser access without SAP GUI
- fast validation of whether curated SQL is enough for the business questions
- no need to build a custom frontend before proving the data layer

## Limits

- this does not replicate SAP transactions
- this does not recreate SAP authorization behavior
- this should stay on curated transparent-table analytics surfaces
