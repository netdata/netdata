//go:build !cgo
// +build !cgo

// SPDX-License-Identifier: GPL-3.0-or-later

package odbcbridge

import (
	"context"
	"database/sql/driver"
	"errors"
)

// Stub implementation for when CGO is disabled

type OptimizedConnection struct{}

func ConnectOptimized(dsn string) (*OptimizedConnection, error) {
	return nil, errors.New("ODBC bridge requires CGO support")
}

func (c *OptimizedConnection) PrepareContext(ctx context.Context, query string) (*PreparedStatement, error) {
	return nil, errors.New("ODBC bridge requires CGO support")
}

func (c *OptimizedConnection) QueryContext(ctx context.Context, query string) (*OptimizedRows, error) {
	return nil, errors.New("ODBC bridge requires CGO support")
}

func (c *OptimizedConnection) Close() error {
	return errors.New("ODBC bridge requires CGO support")
}

func (c *OptimizedConnection) IsConnected() bool {
	return false
}

type PreparedStatement struct{}

func (s *PreparedStatement) Execute() (*OptimizedRows, error) {
	return nil, errors.New("ODBC bridge requires CGO support")
}

func (s *PreparedStatement) Close() error {
	return errors.New("ODBC bridge requires CGO support")
}

type OptimizedRows struct{}

func (r *OptimizedRows) Columns() []string {
	return nil
}

func (r *OptimizedRows) Close() error {
	return errors.New("ODBC bridge requires CGO support")
}

func (r *OptimizedRows) Next(dest []driver.Value) error {
	return errors.New("ODBC bridge requires CGO support")
}

func (r *OptimizedRows) RowCount() int64 {
	return 0
}