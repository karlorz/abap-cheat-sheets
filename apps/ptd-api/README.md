# PTD API

Minimal read-only dataset API for the `PTD_READONLY` dashboard contract catalog.

## Scope

This service does three things:

1. loads dataset contracts from `files/dashboard/contracts`
2. exposes metadata endpoints for those datasets
3. optionally executes a contract-backed SQL dataset when `PTD_SQLSERVER_DSN` is configured

Current limitation:

- `fi-origin-monthly` supports:
  - `posting_from`
  - `posting_to`
  - `company_code`
  - `fi_origin`
- `ar-open-monthly` supports:
  - `posting_from`
  - `posting_to`
  - `company_code`
  - `currency_code`
  - `document_type`
- `ar-cleared-monthly` supports:
  - `clearing_from`
  - `clearing_to`
  - `company_code`
  - `original_document_type`
  - `clearing_document_type`
- `purchasing-history-monthly` supports:
  - `posting_from`
  - `posting_to`
  - `history_category`
  - `plant_code`
- `inventory-movement-monthly` supports:
  - `posting_from`
  - `posting_to`
  - `plant_code`
  - `movement_type`
- `current-stock` supports:
  - `plant_code`
  - `storage_location`
- `dry_run=true` returns the generated SQL and bound args without executing against SQL Server

That boundary is deliberate. The service does not accept arbitrary SQL.

## Run

From repository root:

```bash
cd apps/ptd-api
go run ./cmd/ptd-api
```

Optional environment variables:

```bash
PTD_ADDR=:8080
PTD_REPO_ROOT=/path/to/repo
PTD_CONTRACT_DIR=/path/to/contracts
PTD_QUERY_TIMEOUT=30s

# macOS example: read the PTD SQL login from Keychain and URL-encode it for the DSN
PW="$(security find-generic-password -w -a ptd_reader -s 'ptd_reader@msi-1')"
ENC_PW="$(python3 -c 'import sys, urllib.parse; print(urllib.parse.quote(sys.argv[1], safe=""))' "$PW")"
PTD_SQLSERVER_DSN="sqlserver://ptd_reader:${ENC_PW}@msi-1:1433?database=PTD_READONLY&encrypt=disable"
```

If `PTD_SQLSERVER_DSN` is not set, metadata endpoints still work and dataset execution returns a structured `503`.

## Endpoints

- `GET /healthz`
- `GET /api/datasets`
- `GET /api/datasets/{id}`
- `GET /api/datasets/{id}/rows?limit=100`
- `GET /api/datasets/{id}/rows?dry_run=true`

## Example

```bash
curl http://localhost:8080/api/datasets
curl 'http://localhost:8080/api/datasets/fi-origin-monthly/rows?limit=100'
curl 'http://localhost:8080/api/datasets/fi-origin-monthly/rows?dry_run=true&posting_from=202602&company_code=1000'
curl 'http://localhost:8080/api/datasets/current-stock/rows?dry_run=true&plant_code=6110&storage_location=3G&limit=5'
```
