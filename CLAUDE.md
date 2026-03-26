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
- **Deploy**: Single binary via Go `embed` — contracts, SQL files, and React `dist/` baked in. SSH + run on msi-1.
- **Static serving**: Go serves React dist/ on the same port as the API. No separate web server.

## Known gaps (unresolved)

- **msi-1 is off.** Live SQL execution blocked until the Windows host is running with PTD_READONLY restored.
- **MANDT unverified.** Haven't run `SELECT DISTINCT MANDT` on key tables. Could miss a second production client.
- **Currency decimals.** SQL casts all amounts to `DECIMAL(17,2)`. If data contains JPY (0 dec) or KWD (3 dec), amounts may be wrong. Check `TCURX` table when msi-1 is available.
- **Data scale unknown.** No row-count profiles for BKPF, BSID, EKBE, etc. GROUP BY queries may timeout on large tables.
- **Data freshness undecided.** Don't know if the backup will be re-restored periodically. Cache flush strategy TBD.
- **Go embed not yet implemented.** Binary still discovers files via PTD_REPO_ROOT at runtime.
- **Bearer token not yet implemented.** API currently has zero auth.
- **In-process cache not yet implemented.** API hits SQL on every request.

## Env vars

| Var | Default | Purpose |
|-----|---------|---------|
| `PTD_ADDR` | `:8080` | API listen address |
| `PTD_REPO_ROOT` | auto-discovered via `.git` | Repo root (will be replaced by embed) |
| `PTD_CONTRACT_DIR` | `files/dashboard/contracts` | Contract directory (relative to repo root) |
| `PTD_QUERY_TIMEOUT` | `30s` | Per-query SQL timeout |
| `PTD_SQLSERVER_DSN` | _(none)_ | SQL Server connection string; omit for dry-run only mode |

## Datasets (6 active)

| ID | Domain | Base tables | Key filters |
|----|--------|-------------|-------------|
| fi-origin-monthly | FI | BKPF + T001 | posting_from/to, company_code, fi_origin |
| ar-open-monthly | AR | BSID + T001 | posting_from/to, company_code, currency_code, document_type |
| ar-cleared-monthly | AR | BSAD + BKPF + T001 | clearing_from/to, company_code, original/clearing_document_type |
| purchasing-history-monthly | MM | EKBE + T001W | posting_from/to, history_category, plant_code |
| inventory-movement-monthly | MM | MKPF + MSEG + T001W | posting_from/to, plant_code, movement_type |
| current-stock | MM | MARD + T001W | plant_code, storage_location |

## When msi-1 comes back — validation checklist

1. `SELECT DISTINCT MANDT FROM BKPF` — confirm client 200 is the only production client
2. `SELECT COUNT(*) FROM BKPF WHERE MANDT = '200'` — profile data scale
3. `SELECT DISTINCT WAERS FROM BSID WHERE MANDT = '200'` — check currency exposure for DECIMAL(17,2) risk
4. Run each of the 6 dataset queries via `dry_run=true` then with live SQL — compare result shapes
5. Time the slowest GROUP BY queries — tune `PTD_QUERY_TIMEOUT` if needed
