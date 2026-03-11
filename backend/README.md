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

- The current startup flow initializes the baseline schema before serving requests. Dedicated SQL migrations should still live under `migrations/` as the project evolves.
- Storage bootstrap verifies directory existence and read/write access for `repository`, `staging`, and `trash`.
- On the first successful startup with an empty `admins` table for `super_admin`, the server creates a default `superadmin` account and prints the generated password once in the backend log.
- Management sessions are stored persistently in SQLite table `admin_sessions`; browser refresh and service restart keep sessions valid until `expires_at`.
- Cookie policy is fully config-driven: `name`, `path`, `max_age_seconds`, `renew_window_seconds`, `same_site`, `http_only`, and `secure`.
