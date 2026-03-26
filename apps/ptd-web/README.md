# PTD Web

Minimal React dataset explorer for the `ptd-api` service.

## Purpose

This app is not the final business dashboard. It is the first browser consumer for the
contract-backed API.

It does four things:

1. lists datasets from `GET /api/datasets`
2. shows dataset metadata from `GET /api/datasets/{id}`
3. lets you fill supported filter inputs
4. previews generated SQL through `dry_run=true`

## Run

Start the API first:

```bash
cd apps/ptd-api
GOTOOLCHAIN=local go run ./cmd/ptd-api
```

Then start the web app:

```bash
cd apps/ptd-web
npm install
npm run dev
```

Open:

```text
http://localhost:5173
```

The Vite dev server proxies `/api` to `http://localhost:8080` by default.

If your API runs elsewhere:

```bash
PTD_API_ORIGIN=http://localhost:9090 npm run dev
```
