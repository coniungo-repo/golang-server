# Migrations

SQL migrations for the app database, applied in order by [goose](https://github.com/pressly/goose). They are embedded in the image, and Fly runs `/migrate` as the release command before any machine starts a new version: a failed migration stops the deploy.

```bash
make migration name=create_bookings   # new numbered file here
make migrate                          # apply locally (same code as production)
make migrate-down                     # roll back the last one locally
make generate                         # after changing queries/, regenerate internal/store
```

Rules:

- **Backward compatible.** The old version keeps serving while the new one rolls out, so add before you remove: new columns are nullable or have a default, and old ones are dropped in a later deploy once nothing reads them.
- **Fast under load.** Migrations run with `lock_timeout=10s`; one that waits longer on a busy table fails rather than stalling live queries. Build big indexes with `CREATE INDEX CONCURRENTLY` in their own migration (goose: `-- +goose NO TRANSACTION`).
- **Never edit a migration that has run anywhere.** Write a new one.
- Every migration needs a working `-- +goose Down`.

`TestEmbedsEveryMigration` checks every file here is in the image.
