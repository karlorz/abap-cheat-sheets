# PTD Dataset Contracts

This folder is the seed contract catalog for a future read-only dataset API.

Each dataset has:

- one SQL file in `files/dashboard/catalog`
- one JSON contract in `files/dashboard/contracts`

The contract files are design metadata. They do not mean filtering or paging is already implemented in SQL. The future API layer should:

1. load the contract
2. load the SQL file
3. validate allowed filters
4. inject only bounded, approved predicates
5. execute with hard row and timeout limits

Non-negotiable rules:

- no arbitrary SQL from users or agents
- no unbounded table browsing
- keep `MANDT = '200'` explicit
- stay on validated transparent-table surfaces
