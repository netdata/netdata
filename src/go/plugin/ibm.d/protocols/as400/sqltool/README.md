# ibmi-sql helper (internal tooling)

This directory contains `ibmi-sql`, a small **internal-only** diagnostic utility
for running ad-hoc SQL statements against IBM i using the same ODBC connection
stack as `ibm.d.plugin`.

> **Important:** The binary is excluded from production builds. It is only
> compiled when the custom `internaltools` build tag is provided, ensuring it is
> never shipped with Netdata.

## Building

```bash
cd src/go
# Compile helper into the sqltool directory
go build -tags internaltools -o plugin/ibm.d/protocols/as400/sqltool/ibmi-sql ./plugin/ibm.d/protocols/as400/sqltool
```

## Usage

```bash
plugin/ibm.d/protocols/as400/sqltool/ibmi-sql -config /etc/netdata/ibm.d/as400.conf \
  -job pub400 \                        # job name from the configuration
  -json \                              # optional: emit JSON instead of key/value lines
  "SELECT * FROM QSYS2.MESSAGE_QUEUE_INFO FETCH FIRST 5 ROWS ONLY"
```

Key notes:
- `-config` is required. It must point to an AS/400 job configuration file (same
  format the plugin uses).
- `-job` selects which job entry to reuse (DSN/hostname/password/etc.).
- Input SQL is passed as a single positional argument.
- `-timeout` (default 30s) bounds overall execution.
- `-json` formats rows as a JSON array; when omitted, results are printed as
  key/value pairs separated by `--` per row.

## Output Examples

JSON output (`-json`):
```json
[
  {"MESSAGE_QUEUE_LIBRARY":"QSYS","MESSAGE_QUEUE_NAME":"#","MESSAGE_TYPE":"INFORMATIONAL","SEVERITY":"99"}
]
```

Text output (default):
```
MESSAGE_QUEUE_LIBRARY=QSYS
MESSAGE_QUEUE_NAME=#
MESSAGE_TYPE=INFORMATIONAL
SEVERITY=99
--
```

## Cleanup

The `.gitignore` in this directory ensures local builds (`ibmi-sql`) are not
tracked. Remove binaries manually when finished (`rm ibmi-sql`).

## Use Cases

- Quickly inspect SQL service schemas (e.g. `QSYS2.MESSAGE_QUEUE_INFO`).
- Harvest sample rows for new collectors (message queues, output queues, etc.).
- Verify SQL services availability on specific partitions (feature detection).

> The tool intentionally avoids any automation around feature detection or
> collector wiring; it’s purely a developer helper for manual inspection.
