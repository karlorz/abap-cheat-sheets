# SAP

## Repo layout

```
apps/ptd-api/          Go read-only dataset API (cmd/ptd-api, internal/{config,catalog,server})
apps/ptd-web/          React explorer SPA (Vite + JSX)
files/dashboard/
  contracts/           JSON dataset contracts (one per dataset)
  catalog/             SQL files (one per contract)
```

## Key commands

```bash
# API — build & test
cd apps/ptd-api && GOTOOLCHAIN=local go test ./...
cd apps/ptd-api && GOTOOLCHAIN=local go build -o ptd-api ./cmd/ptd-api
# Validate data assumptions on msi-1 (auth token not required for --validate)
cd apps/ptd-api && PTD_SQLSERVER_DSN="sqlserver://ptd_reader:password@msi-1:1433?database=PTD_READONLY&encrypt=disable" GOTOOLCHAIN=local go run ./cmd/ptd-api --validate

# Web — build & dev
cd apps/ptd-web && npm install && npm run build
cd apps/ptd-web && npm run dev          # dev server on :5173, proxies /api → :8080
```

## Non-negotiable rules

1. **No arbitrary SQL from users or agents.** Only predefined contract queries with whitelisted filters.
2. **MANDT = '200' in every query.** Hardcoded in SQL files, not a user-supplied parameter.
3. **Read-only.** No INSERT/UPDATE/DELETE. All contracts marked `read_only: true`.
4. **Parameterized queries only.** All filter values bound as `@p1, @p2, ...` — never string-concatenated.
5. **Row limits enforced.** Max 5000 per contract; API rejects requests exceeding the cap.

## Architecture decisions (settled)

- **Auth**: Static bearer token (env var) checked in Go middleware. Required before enabling live SQL.
- **Caching**: In-process TTL cache in Go server, keyed by dataset+filters. Contract `cache_ttl_seconds` drives eviction.
- **Warmup**: On boot, when `PTD_SQLSERVER_DSN` is set, the API pre-executes all 6 datasets with no filters and seeds the response cache. Failures are logged and skipped.
- **Deploy**: Single binary via Go `embed` — contracts, SQL files, and React `dist/` baked in. SSH + run on msi-1.
- **Static serving**: Go serves React dist/ on the same port as the API. No separate web server.

## Known gaps (unresolved)

- **MANDT mismatch is real.** Live validation on `msi-1` shows `ptd.BKPF` contains clients `050` and `200`. The `050` slice is small and old (`75` BKPF rows from `2009-02-01` to `2009-04-07`, mostly company codes `1000` and `6200`), while `200` is the dominant live client (`12,815` BKPF rows through `2026-02-13`). Dataset SQL still hardcodes `MANDT = '200'`, and `--validate` intentionally fails until the extra client is reviewed.
- **Currency decimals remain a known limitation.** Live validation shows `JPY` in both `BSID` and `BSAD`, so `DECIMAL(17,2)` is lossy for at least one active 0-decimal currency. `KWD` and `BHD` were not observed in AR data, and `TCURX` is not present in the restored `PTD_READONLY` schema, so the decimal configuration cannot be confirmed from this copy. This is still ignored for the pilot.
- **Data freshness undecided.** Don't know if the backup will be re-restored periodically. Cache flush strategy TBD.

## Resolved

- **msi-1 is online.** Live SQL validation can run from the dev laptop via SQL login `ptd_reader`.
- **Live SQL transport is enabled.** SQL Server mixed mode and TCP/IP are enabled, and `msi-1:1433` accepts remote SQL connections.
- **Go embed is implemented.** The API can serve embedded contracts, SQL files, and web assets from a single binary.
- **Bearer token is implemented.** Server mode requires `PTD_AUTH_TOKEN` whenever `PTD_SQLSERVER_DSN` is set.
- **In-process cache is implemented.** Dataset row responses are cached by dataset+filters with TTL from each contract.
- **Validation entrypoint exists.** Run `go run ./cmd/ptd-api --validate` with `PTD_SQLSERVER_DSN` to check schema, `MANDT`, row counts, currencies, and dataset timings.
- **Warmup is verified.** Booting the API with `PTD_SQLSERVER_DSN` preloads all 6 datasets, and the first unfiltered request returns `X-Cache: HIT`.

## Env vars

| Var | Default | Purpose |
|-----|---------|---------|
| `PTD_ADDR` | `:8080` | API listen address |
| `PTD_REPO_ROOT` | auto-discovered via `.git` | Repo root for local filesystem mode; embedded builds do not need it |
| `PTD_CONTRACT_DIR` | `files/dashboard/contracts` | Contract directory (relative to repo root) |
| `PTD_QUERY_TIMEOUT` | `30s` | Per-query SQL timeout |
| `PTD_SQLSERVER_DSN` | _(none)_ | SQL Server connection string; omit for dry-run only mode |

Warmup runs automatically when `PTD_SQLSERVER_DSN` is set. There is no separate `PTD_WARMUP` env var.

## Datasets (6 active)

| ID | Domain | Base tables | Key filters |
|----|--------|-------------|-------------|
| fi-origin-monthly | FI | BKPF + T001 | posting_from/to, company_code, fi_origin |
| ar-open-monthly | AR | BSID + T001 | posting_from/to, company_code, currency_code, document_type |
| ar-cleared-monthly | AR | BSAD + BKPF + T001 | clearing_from/to, company_code, original/clearing_document_type |
| purchasing-history-monthly | MM | EKBE + T001W | posting_from/to, history_category, plant_code |
| inventory-movement-monthly | MM | MKPF + MSEG + T001W | posting_from/to, plant_code, movement_type |
| current-stock | MM | MARD + T001W | plant_code, storage_location |

## Live SQL validation findings

1. `SELECT DISTINCT MANDT FROM ptd.BKPF` returns `050` and `200`, so the validation command currently fails only on the MANDT check.
2. The `050` slice appears small and historical rather than co-equal with `200`: `BKPF 75`, `BSID 5`, `BSAD 2`, `EKBE 7`, `MKPF 15`, `MSEG 16`, `MARD 5`; `BKPF` dates run from `2009-02-01` to `2009-04-07`, versus `200` rows extending to `2026-02-13`.
3. Row counts for `MANDT = '200'`: `BKPF 12815`, `BSID 1651`, `BSAD 1381`, `EKBE 5254`, `MKPF 6468`, `MSEG 9957`, `MARD 1548`, `T001 29`, `T001W 15`.
4. AR currencies in live `MANDT = '200'` data are:
   `BSID`: `CHF, EUR, GBP, HKD, JPY, RMB, USD, VND`
   `BSAD`: `CHF, EUR, HKD, JPY, RMB, USD`
   This confirms an active JPY precision risk; `KWD` and `BHD` were not observed.
5. `TCURX` is not present in the restored `PTD_READONLY` schema, so decimal metadata cannot be confirmed from this database copy.
6. All 6 dataset queries execute successfully in live mode with `--validate`; recent `TOP 10` timings were about `0.02s` to `0.10s`.
