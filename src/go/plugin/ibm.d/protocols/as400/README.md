# IBM i (AS/400) protocol helpers

This package implements the low-level ODBC helper used by the AS/400 collector.

## Files

- `client.go` — production client (requires CGO/ODBC).
- `client_stub.go` — build stub for non-CGO platforms.

## Usage

The collector imports this package to issue SQL statements and handle error
classification. No standalone tooling ships from this directory; the code is
intended for internal use by the AS/400 module.
