// Package odbcbridge provides an optimized ODBC connection interface using CGO.

//go:build cgo
// +build cgo

package odbcbridge

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"sync"
)

func init() {
	sql.Register("odbcbridge", &ODBCDriver{})
}

// ODBCDriver implements database/sql/driver.Driver
type ODBCDriver struct{}

// Open returns a new connection to the database
func (d *ODBCDriver) Open(dsn string) (driver.Conn, error) {
	conn, err := ConnectOptimized(dsn)
	if err != nil {
		return nil, err
	}
	return &driverConn{conn: conn}, nil
}

// driverConn implements driver.Conn
type driverConn struct {
	conn *OptimizedConnection
	mu   sync.Mutex
}

// Prepare returns a prepared statement
func (dc *driverConn) Prepare(query string) (driver.Stmt, error) {
	return dc.PrepareContext(context.Background(), query)
}

// PrepareContext returns a prepared statement
func (dc *driverConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	stmt, err := dc.conn.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return &driverStmt{stmt: stmt, conn: dc}, nil
}

// Close closes the connection
func (dc *driverConn) Close() error {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	return dc.conn.Close()
}

// Begin starts and returns a new transaction
func (dc *driverConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("transactions not implemented")
}

// BeginTx starts and returns a new transaction
func (dc *driverConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return nil, fmt.Errorf("transactions not implemented")
}

// QueryContext executes a query that returns rows
func (dc *driverConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if len(args) > 0 {
		return nil, fmt.Errorf("query arguments not supported for direct queries, use prepared statements")
	}

	rows, err := dc.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return &driverRows{rows: rows}, nil
}

// ExecContext executes a query that doesn't return rows
func (dc *driverConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if len(args) > 0 {
		return nil, fmt.Errorf("exec arguments not supported for direct queries, use prepared statements")
	}

	rows, err := dc.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Get row count for result
	rowCount := rows.RowCount()
	return &driverResult{rowsAffected: rowCount}, nil
}

// Ping verifies a connection to the database is still alive
func (dc *driverConn) Ping(ctx context.Context) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if !dc.conn.IsConnected() {
		return fmt.Errorf("connection is closed")
	}

	// Execute a simple query to verify connection
	rows, err := dc.conn.QueryContext(ctx, "SELECT 1 FROM SYSIBM.SYSDUMMY1")
	if err != nil {
		return err
	}
	rows.Close()
	return nil
}

// driverStmt implements driver.Stmt
type driverStmt struct {
	stmt *PreparedStatement
	conn *driverConn
}

// Close closes the statement
func (ds *driverStmt) Close() error {
	return ds.stmt.Close()
}

// NumInput returns the number of placeholder parameters
func (ds *driverStmt) NumInput() int {
	// ODBC doesn't provide this information easily
	return -1
}

// Exec executes a prepared statement
func (ds *driverStmt) Exec(args []driver.Value) (driver.Result, error) {
	return nil, fmt.Errorf("Exec not implemented, use ExecContext")
}

// ExecContext executes a prepared statement
func (ds *driverStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	// For now, execute without parameters
	rows, err := ds.stmt.Execute()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rowCount := rows.RowCount()
	return &driverResult{rowsAffected: rowCount}, nil
}

// Query executes a prepared statement and returns rows
func (ds *driverStmt) Query(args []driver.Value) (driver.Rows, error) {
	return nil, fmt.Errorf("Query not implemented, use QueryContext")
}

// QueryContext executes a prepared statement and returns rows
func (ds *driverStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	// For now, execute without parameters
	rows, err := ds.stmt.Execute()
	if err != nil {
		return nil, err
	}
	return &driverRows{rows: rows}, nil
}

// driverRows implements driver.Rows
type driverRows struct {
	rows   *OptimizedRows
	closed bool
}

// Columns returns the column names
func (dr *driverRows) Columns() []string {
	return dr.rows.Columns()
}

// Close closes the rows
func (dr *driverRows) Close() error {
	if dr.closed {
		return nil
	}
	dr.closed = true
	return dr.rows.Close()
}

// Next advances to the next row
func (dr *driverRows) Next(dest []driver.Value) error {
	if dr.closed {
		return io.EOF
	}
	return dr.rows.Next(dest)
}

// driverResult implements driver.Result
type driverResult struct {
	rowsAffected int64
}

// LastInsertId returns the last inserted ID
func (dr *driverResult) LastInsertId() (int64, error) {
	return 0, fmt.Errorf("LastInsertId not supported")
}

// RowsAffected returns the number of rows affected
func (dr *driverResult) RowsAffected() (int64, error) {
	return dr.rowsAffected, nil
}
