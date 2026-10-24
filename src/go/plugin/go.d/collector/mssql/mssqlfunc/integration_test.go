// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package mssqlfunc

import (
	"os"
	"testing"

	_ "github.com/microsoft/go-mssqldb"
)

func getDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("MSSQL_DSN")
	if dsn == "" {
		t.Skip("MSSQL_DSN environment variable not set")
	}
	return dsn
}
