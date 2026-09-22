// SPDX-License-Identifier: GPL-3.0-or-later

// Package mssqlfunc implements the MSSQL diagnostic Functions.
package mssqlfunc

import (
	"context"
	"database/sql"
)

// Queryer exposes reads through the collector-owned Function connection pool.
type Queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// ServerInfo is the server information needed to select diagnostic SQL and permissions.
type ServerInfo struct {
	MajorVersion     int
	AzureSQLDatabase bool
}

// Deps provides the Function pool and the server information shared with metric collection.
// DB returns nil before initialization. EnsureServerInfo uses the supplied context to load
// missing information; ServerInfo returns a synchronized snapshot of the current values.
// The collector owns connection lifetime; Functions never open or close a pool.
type Deps interface {
	DB() Queryer
	ServerInfo() ServerInfo
	EnsureServerInfo(context.Context) error
}
