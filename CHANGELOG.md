# Changelog

All notable changes to this project will be documented in this file.

## [1.3.1] - 2026-05-10

### Added
- New API action `reset-and-import-config-to-db` to reset database and import all existing .caddy files
- QEMU support in GitHub Actions for ARM64 cross-compilation
- `.env` file support in installer script with default configuration

### Fixed
- Fixed soft delete logic to properly check for existing non-deleted domains and config files
- Fixed duplicate domain validation to work with soft delete system
- Resolved GoReleaser configuration compatibility issues
- Fixed Go version compatibility (downgraded to 1.22 for broader compatibility)

### Changed
- Disabled ARM64 builds temporarily due to sqlite3 CGO compilation issues on darwin_arm64
- Updated installer to create `.env` file with proper port configuration (1011)
- Improved error handling in domain addition actions

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
