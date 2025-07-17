# ODBC Bridge Package

## Overview

The `odbcbridge` package provides a reliable ODBC connection interface for Go applications, specifically designed to handle AS/400 and other enterprise database systems. It solves critical issues with existing Go ODBC drivers through a direct C bridge implementation.

## Why This Package Exists

### The Problem

The popular `alexbrainman/odbc` Go driver has a critical bug where it doesn't properly clean up prepared statement handles when queries fail. This causes subsequent queries to fail with SQL0519 errors ("Prepared statement in use"), effectively breaking the connection until reconnection.

### The Solution

This package implements a C-based ODBC bridge that:

1. **Always cleans up statement handles** - Even when queries fail
2. **Prevents SQL0519 errors** - Proper statement lifecycle management
3. **Optimizes performance** - Statement reuse with proper reset
4. **Handles AS/400 quirks** - Negative row counts, special data types

## Features

- **Reliable Statement Management**: Prevents SQL0519 errors through proper cleanup
- **Statement Reuse**: Pre-allocated statement handles for better performance
- **AS/400 Support**: Handles negative row counts and special behaviors
- **Type-Safe**: Proper handling of INT64, DOUBLE, STRING, and BINARY types
- **Context Support**: Full context.Context integration for cancellation
- **Thread-Safe**: Mutex protection for concurrent access
- **Error Recovery**: Automatic statement reset on errors

## Architecture

```
Go Application
    ↓
connection.go (Go interface)
    ↓
CGO Bridge
    ↓
bridge.c (C implementation)
    ↓
ODBC Driver (unixODBC/iODBC)
    ↓
Database
```

## Usage

### Basic Connection

```go
import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/pkg/odbcbridge"

// Connect to database
conn, err := odbcbridge.ConnectOptimized(dsn)
if err != nil {
    return err
}
defer conn.Close()

// Execute query
rows, err := conn.QueryContext(ctx, "SELECT * FROM QSYS2.SYSTEM_STATUS_INFO")
if err != nil {
    return err
}
defer rows.Close()

// Process results
columns := rows.Columns()
values := make([]driver.Value, len(columns))

for rows.Next(values) == nil {
    // Process row
    for i, v := range values {
        fmt.Printf("%s: %v\n", columns[i], v)
    }
}
```

### Prepared Statements

```go
// Prepare statement
stmt, err := conn.PrepareContext(ctx, "SELECT * FROM TABLE WHERE ID = ?")
if err != nil {
    return err
}
defer stmt.Close()

// Execute multiple times
for _, id := range ids {
    rows, err := stmt.Execute()
    if err != nil {
        continue // Safe - no SQL0519!
    }
    // Process rows...
    rows.Close()
}
```

### Type-Safe Scanning

```go
var (
    name  string
    count int64
    ratio float64
)

err := rows.ScanTyped(&name, &count, &ratio)
if err != nil {
    return err
}
```

## Implementation Details

### Statement Lifecycle

1. **Connection**: Pre-allocates a statement handle for reuse
2. **Query Execution**: Resets statement if needed, executes query
3. **Error Handling**: Always resets statement on error to prevent SQL0519
4. **Cursor Management**: Properly closes cursors between queries
5. **Cleanup**: Frees all resources on connection close

### Data Type Handling

The bridge automatically detects SQL types and converts them appropriately:

- `SQL_INTEGER`, `SQL_BIGINT` → `int64`
- `SQL_FLOAT`, `SQL_DOUBLE`, `SQL_DECIMAL` → `float64`
- `SQL_CHAR`, `SQL_VARCHAR` → `string`
- `SQL_BINARY`, `SQL_VARBINARY` → `[]byte`

### AS/400 Specific Handling

- **Negative Row Counts**: Returns `int64` (not unsigned) to handle AS/400's negative row counts
- **String Conversions**: Handles EBCDIC conversions through the ODBC driver
- **Special SQL Types**: Proper handling of AS/400-specific data types

## Performance Considerations

1. **Statement Reuse**: Pre-allocated statement handles reduce allocation overhead
2. **Buffer Management**: Reuses internal buffers for column data
3. **Minimal CGO Calls**: Batches operations where possible
4. **Connection Pooling**: Designed to work with connection pools

## Error Handling

The bridge provides detailed error information:

```go
rows, err := conn.QueryContext(ctx, query)
if err != nil {
    // Error includes SQLSTATE and native error codes
    // Example: "SQLExecDirect: 42S02:1:-204:[IBM][System i Access ODBC Driver]
    //          [DB2 for i5/OS]SQL0204 - INVALID_TABLE in QSYS2 type *FILE not found."
    log.Printf("Query failed: %v", err)
}
```

## Building

Requires:
- C compiler (gcc/clang)
- ODBC development headers (`unixodbc-dev` on Ubuntu/Debian)
- CGO enabled

```bash
# Install dependencies (Ubuntu/Debian)
sudo apt-get install unixodbc-dev

# Build
go build -tags cgo
```

## Testing

```go
// Run tests
go test -v ./...

// Test with specific DSN
DSN="Driver={IBM i Access ODBC Driver};System=pub400.com;..." go test -v
```

## Comparison with alexbrainman/odbc

| Feature | alexbrainman/odbc | odbcbridge |
|---------|-------------------|------------|
| SQL0519 Prevention | ❌ Bug causes SQL0519 | ✅ Proper cleanup |
| Statement Reuse | ❌ Creates new each time | ✅ Optimized reuse |
| AS/400 Row Counts | ❌ uint64 (wrong for negative) | ✅ int64 (correct) |
| Error Recovery | ❌ Connection unusable | ✅ Auto-recovery |
| Memory Leaks | ❌ Known issues | ✅ Proper cleanup |
| Context Support | ✅ Yes | ✅ Yes |
| Type Safety | ⚠️ Limited | ✅ Full type info |

## License

Same as Netdata (GPL-3.0-or-later)