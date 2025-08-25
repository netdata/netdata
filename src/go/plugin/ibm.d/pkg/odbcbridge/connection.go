// Package odbcbridge provides an optimized ODBC connection interface using CGO.
// This version handles AS400-specific issues like negative row counts and proper data types.
package odbcbridge

/*
#cgo CFLAGS: -I.
#cgo LDFLAGS: -lodbc
#include "bridge.h"
#include <stdlib.h>
*/
import "C"
import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"unsafe"
)

// OptimizedConnection represents an ODBC database connection with statement reuse
type OptimizedConnection struct {
	handle C.odbc_conn_t
	mu     sync.Mutex
}

// ConnectOptimized establishes a new optimized ODBC connection
func ConnectOptimized(dsn string) (*OptimizedConnection, error) {
	cDSN := C.CString(dsn)
	defer C.free(unsafe.Pointer(cDSN))

	errorBuf := make([]byte, 1024)
	handle := C.odbc_connect(cDSN, (*C.char)(unsafe.Pointer(&errorBuf[0])), C.int(len(errorBuf)))

	if handle == nil {
		return nil, fmt.Errorf("connection failed: %s", string(errorBuf))
	}

	return &OptimizedConnection{handle: handle}, nil
}

// Close closes the connection
func (c *OptimizedConnection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.handle != nil {
		C.odbc_disconnect(c.handle)
		c.handle = nil
	}
	return nil
}

// IsConnected checks if the connection is active
func (c *OptimizedConnection) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.handle == nil {
		return false
	}
	return C.odbc_is_connected(c.handle) != 0
}

// QueryContext executes a query and returns optimized rows
func (c *OptimizedConnection) QueryContext(ctx context.Context, query string) (*OptimizedRows, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.handle == nil {
		return nil, ErrNotConnected
	}

	// Check context before executing
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	cQuery := C.CString(query)
	defer C.free(unsafe.Pointer(cQuery))

	errorBuf := make([]byte, 1024)

	// Use execute_direct for one-off queries (optimizes for AS400)
	ret := C.odbc_execute_direct(c.handle, cQuery, (*C.char)(unsafe.Pointer(&errorBuf[0])), C.int(len(errorBuf)))

	if ret != Success {
		return nil, fmt.Errorf("%w: %s", ErrQueryFailed, string(errorBuf))
	}

	// Get metadata
	columnCount := int(C.odbc_get_column_count(c.handle))
	if columnCount == 0 {
		C.odbc_reset_statement(c.handle)
		return nil, ErrNoRows
	}

	// Get row count (can be negative on AS400!)
	rowCount := int64(C.odbc_get_row_count(c.handle))

	// Get column info
	columns := make([]ColumnInfo, columnCount)
	for i := 0; i < columnCount; i++ {
		var cInfo C.odbc_column_info_t
		if C.odbc_get_column_info(c.handle, C.int(i), &cInfo) == Success {
			columns[i] = ColumnInfo{
				Name:     C.GoString(&cInfo.name[0]),
				DataType: DataType(cInfo._type),
				SQLType:  int(cInfo.sql_type),
				Size:     int(cInfo.size),
				Scale:    int(cInfo.scale),
				Nullable: bool(cInfo.nullable),
			}
		}
	}

	return &OptimizedRows{
		conn:     c,
		columns:  columns,
		rowCount: rowCount,
		ctx:      ctx,
	}, nil
}

// PrepareContext prepares a statement for repeated execution
func (c *OptimizedConnection) PrepareContext(ctx context.Context, query string) (*PreparedStatement, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.handle == nil {
		return nil, ErrNotConnected
	}

	cQuery := C.CString(query)
	defer C.free(unsafe.Pointer(cQuery))

	errorBuf := make([]byte, 1024)
	ret := C.odbc_prepare(c.handle, cQuery, (*C.char)(unsafe.Pointer(&errorBuf[0])), C.int(len(errorBuf)))

	if ret != Success {
		return nil, fmt.Errorf("prepare failed: %s", string(errorBuf))
	}

	return &PreparedStatement{conn: c, ctx: ctx}, nil
}

// Return codes from C bridge
const (
	Success = C.ODBC_SUCCESS
	Error   = C.ODBC_ERROR
	NoData  = C.ODBC_NO_DATA
)

// Common errors
var (
	ErrNotConnected = errors.New("not connected")
	ErrQueryFailed  = errors.New("query failed")
	ErrNoRows       = errors.New("no rows returned")
)

// DataType represents ODBC data types
type DataType int

const (
	TypeNull   DataType = C.ODBC_TYPE_NULL
	TypeInt64  DataType = C.ODBC_TYPE_INT64
	TypeDouble DataType = C.ODBC_TYPE_DOUBLE
	TypeString DataType = C.ODBC_TYPE_STRING
	TypeBinary DataType = C.ODBC_TYPE_BINARY
)

// ColumnInfo contains column metadata
type ColumnInfo struct {
	Name     string
	DataType DataType
	SQLType  int
	Size     int
	Scale    int
	Nullable bool
}

// OptimizedRows represents the result of a query with proper type handling
type OptimizedRows struct {
	conn     *OptimizedConnection
	columns  []ColumnInfo
	rowCount int64 // Can be negative on AS400!
	ctx      context.Context
	closed   bool
	mu       sync.Mutex
}

// Columns returns the column names
func (r *OptimizedRows) Columns() []string {
	names := make([]string, len(r.columns))
	for i, col := range r.columns {
		names[i] = col.Name
	}
	return names
}

// ColumnInfo returns detailed column information
func (r *OptimizedRows) ColumnInfo() []ColumnInfo {
	return r.columns
}

// RowCount returns the number of rows affected (can be negative on AS400!)
func (r *OptimizedRows) RowCount() int64 {
	return r.rowCount
}

// Next advances to the next row with proper type handling
func (r *OptimizedRows) Next(dest []driver.Value) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return io.EOF
	}

	// Check context
	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	default:
	}

	ret := C.odbc_fetch_row(r.conn.handle)
	if ret == NoData {
		return io.EOF
	} else if ret != Success {
		return errors.New("fetch failed")
	}

	// Get values with proper type conversion
	for i := range dest {
		var value C.odbc_value_t
		if C.odbc_get_value(r.conn.handle, C.int(i), &value) == Success {
			dest[i] = convertValue(&value)
			C.odbc_free_value(&value)
		} else {
			dest[i] = nil
		}
	}

	return nil
}

// ScanTyped scans the current row with type information
func (r *OptimizedRows) ScanTyped(dest ...interface{}) error {
	values := make([]driver.Value, len(dest))
	err := r.Next(values)
	if err != nil {
		return err
	}

	for i, v := range values {
		if err := convertAssignTyped(dest[i], v, r.columns[i].DataType); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the rows
func (r *OptimizedRows) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true
	// Just close cursor, keep statement for reuse
	C.odbc_close_cursor(r.conn.handle)
	return nil
}

// PreparedStatement represents a prepared SQL statement
type PreparedStatement struct {
	conn *OptimizedConnection
	ctx  context.Context
}

// Execute executes the prepared statement
func (s *PreparedStatement) Execute() (*OptimizedRows, error) {
	s.conn.mu.Lock()
	defer s.conn.mu.Unlock()

	errorBuf := make([]byte, 1024)
	ret := C.odbc_execute(s.conn.handle, (*C.char)(unsafe.Pointer(&errorBuf[0])), C.int(len(errorBuf)))

	if ret != Success {
		return nil, fmt.Errorf("execute failed: %s", string(errorBuf))
	}

	// Get metadata (same as QueryContext)
	columnCount := int(C.odbc_get_column_count(s.conn.handle))
	rowCount := int64(C.odbc_get_row_count(s.conn.handle))

	columns := make([]ColumnInfo, columnCount)
	for i := 0; i < columnCount; i++ {
		var cInfo C.odbc_column_info_t
		if C.odbc_get_column_info(s.conn.handle, C.int(i), &cInfo) == Success {
			columns[i] = ColumnInfo{
				Name:     C.GoString(&cInfo.name[0]),
				DataType: DataType(cInfo._type),
				SQLType:  int(cInfo.sql_type),
				Size:     int(cInfo.size),
				Scale:    int(cInfo.scale),
				Nullable: bool(cInfo.nullable),
			}
		}
	}

	return &OptimizedRows{
		conn:     s.conn,
		columns:  columns,
		rowCount: rowCount,
		ctx:      s.ctx,
	}, nil
}

// Close closes the prepared statement
func (s *PreparedStatement) Close() error {
	s.conn.mu.Lock()
	defer s.conn.mu.Unlock()

	C.odbc_reset_statement(s.conn.handle)
	return nil
}

// convertValue converts C value to Go value with proper type handling
func convertValue(cValue *C.odbc_value_t) driver.Value {
	if cValue.is_null {
		return nil
	}

	switch DataType(cValue._type) {
	case TypeInt64:
		return int64(cValue.data[0])
	case TypeDouble:
		// Handle double properly
		doublePtr := (*float64)(unsafe.Pointer(&cValue.data[0]))
		return *doublePtr
	case TypeString:
		// String is stored as a pointer in the union
		strPtr := (*unsafe.Pointer)(unsafe.Pointer(&cValue.data[0]))
		if *strPtr != nil {
			return C.GoString((*C.char)(*strPtr))
		}
		return ""
	default:
		return nil
	}
}

// convertAssignTyped converts a driver.Value to dest with type awareness
func convertAssignTyped(dest interface{}, src driver.Value, dataType DataType) error {
	if src == nil {
		return nil
	}

	switch d := dest.(type) {
	case *string:
		switch s := src.(type) {
		case string:
			*d = s
			return nil
		default:
			*d = fmt.Sprint(src)
			return nil
		}

	case *int64:
		switch dataType {
		case TypeInt64:
			if v, ok := src.(int64); ok {
				*d = v
				return nil
			}
		case TypeString:
			// AS400 might return numbers as strings
			if s, ok := src.(string); ok {
				val, err := strconv.ParseInt(s, 10, 64)
				if err != nil {
					return err
				}
				*d = val
				return nil
			}
		}

	case *float64:
		switch dataType {
		case TypeDouble:
			if v, ok := src.(float64); ok {
				*d = v
				return nil
			}
		case TypeString:
			// AS400 might return decimals as strings
			if s, ok := src.(string); ok {
				val, err := strconv.ParseFloat(s, 64)
				if err != nil {
					return err
				}
				*d = val
				return nil
			}
		}

	case *interface{}:
		*d = src
		return nil
	}

	return fmt.Errorf("unsupported conversion from %T (type %v) to %T", src, dataType, dest)
}
