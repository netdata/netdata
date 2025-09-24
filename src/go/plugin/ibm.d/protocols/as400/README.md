# IBM i (AS/400) protocol helpers

This package implements the low-level ODBC client used by the AS/400 collector.
It exposes helpers for running SQL queries and classified error handling.

## Files

- `client.go` — production client (requires CGO/ODBC).
- `client_stub.go` — build stub for non-CGO platforms.
- `sqltool/` — internal-only tooling; includes `ibmi-sql`, a helper you can build locally (see its README) to execute ad-hoc SQL using the same connection stack as the collector. The helper is compiled only with the `internaltools` build tag and is not shipped with Netdata.

## Internal helper

To build the optional SQL tool:

```bash
cd src/go
go build -tags internaltools -o plugin/ibm.d/protocols/as400/sqltool/ibmi-sql ./plugin/ibm.d/protocols/as400/sqltool
```

See `sqltool/README.md` for usage instructions.
