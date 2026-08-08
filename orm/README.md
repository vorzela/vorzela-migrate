# vorm (Vorzela ORM)

| Folder | Status |
|--------|--------|
| [`go/`](go/README.md) | **Active** — [`LLM.md`](go/LLM.md) |
| [`typescript/`](typescript/) | Planned |
| [`python/`](python/) | Planned |

## Go output (no sqlc)

| Path | Role |
|------|------|
| `vorm/gen/` | Generated Go — **pgx v5** / MySQL drivers via `query.DB` |
| `models/` | Generated models |

Hand-write: `schema/migrations/`, `queries/`. Values always parameterized (no SQL injection). Never `SELECT *`.
