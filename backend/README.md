# OpenShare Backend

## Configuration loading order

The backend resolves configuration in the following order:

1. `configs/config.default.json`
2. `configs/config.local.json` (optional, ignored when absent)
3. Environment variables prefixed with `OPENSHARE_`

## Example environment overrides

- `OPENSHARE_SERVER_PORT=9090`
- `OPENSHARE_DATABASE_PATH=/data/openshare/openshare.db`
- `OPENSHARE_STORAGE_ROOT=/data/openshare`
- `OPENSHARE_SESSION_SECRET=change-me`

## Notes

- SQLite migrations are not driven by `AutoMigrate`; SQL migrations should stay under `migrations/`.
- Storage bootstrap verifies directory existence and read/write access for `repository`, `staging`, and `trash`.
