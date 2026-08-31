# pg-index-translate

Generates PostgreSQL `CREATE INDEX` statements from MySQL `KEY` / `UNIQUE KEY`
declarations in `server/datastore/mysql/schema.sql`. Output is intended to
be embedded by a one-shot migration that brings a fresh PG deployment to
index parity with MySQL.

## Why

The PG baseline schema (`server/datastore/mysql/pg_baseline_schema.sql`)
was originally generated without translating the MySQL `KEY` clauses, so
PG had ~11 indexes vs MySQL's ~354. The migration
`20260513210000_AddMissingPGIndexes` uses this tool's output to close
that gap.

## Usage

```sh
go run ./tools/pg-index-translate \
  -in  server/datastore/mysql/schema.sql \
  -out server/datastore/mysql/migrations/tables/20260513210000_AddMissingPGIndexes.sql
```

The script:

- Emits `CREATE INDEX IF NOT EXISTS …` (or `CREATE UNIQUE INDEX IF NOT EXISTS …`)
  per `KEY` / `UNIQUE KEY` clause, grouped by table for readable diffs.
- Skips `PRIMARY KEY`, `FULLTEXT KEY`, `SPATIAL KEY`, and prefix-length
  indexes (`col(N)`) — these need PG-specific implementations (pg_trgm,
  to_tsvector, expression indexes).
- Preserves `DESC` ordering on individual columns (PG supports it).
- Strips MySQL backticks. Identifiers stay unquoted; the existing PG
  baseline uses unquoted lower-snake identifiers throughout.

Stderr prints a summary of emitted vs skipped, with reasons for each skip.
