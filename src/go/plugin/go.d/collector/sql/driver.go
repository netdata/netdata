// SPDX-License-Identifier: GPL-3.0-or-later

package sql

import (
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/microsoft/go-mssqldb"
	_ "github.com/sijms/go-ora/v2"
)

var supportedDrivers = map[string]bool{
	"mysql":     true,
	"oracle":    true,
	"pgx":       true,
	"sqlserver": true,
}
