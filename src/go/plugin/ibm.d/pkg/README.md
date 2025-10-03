# Shared CGO Packages

The `pkg/` directory houses low-level building blocks that require CGO and are shared by multiple protocols/modules.

Current packages:

| Package | Description |
|---------|-------------|
| `dbdriver/` | Thin wrappers around IBM’s DB2 CLI driver (dynamic loading, environment setup). |
| `odbcbridge/` | Lightweight bridge that exposes ODBC handles to Go code with safe conversions. |
| `odbcwrapper/` | Higher-level helpers for executing queries and scanning results through the bridge. |

These packages are intentionally small and focus on ABI boundaries. Business logic belongs in `protocols/` and `modules/`.

When touching CGO code remember to:

- Keep build tags accurate (`//go:build cgo`).
- Free allocated C resources even on error paths.
- Document any required compile-time flags or environment variables.
