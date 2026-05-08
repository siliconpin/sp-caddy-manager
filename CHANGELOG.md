# Changelog

All notable changes to this project will be documented in this file.

## [1.1.0] - 2026-05-08

### Added
- Store raw Caddyfile snippet in the database `content` column.
- Added `id`, `created_at`, and `updated_at` columns to `domains` table.
- Added `deleted` column and implemented soft-delete behavior for domains.
- New API action `list-db-with-content` to return `id`, `domain`, `port`, `content`, `created_at`, `updated_at` for non-deleted rows.

### Changed
- `DELETE` action now marks rows as deleted and removes on-disk `.caddy` file and Caddy route (soft delete in DB).

### Notes
- No automatic DB migrations are included; existing databases will need to be updated to include the new columns.
