// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows && cgo
// +build !windows,cgo

package dbdriver

import (
	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/pkg/odbcbridge"
)

func init() {
	// Register our optimized ODBC bridge driver
	Register("odbcbridge", &Driver{
		Name:         "odbcbridge",
		Description:  "Optimized ODBC bridge driver (prevents SQL0519, handles AS/400 quirks)",
		Available:    true,
		RequiresCGO:  true,
		RequiresLibs: []string{"unixODBC", "IBM i Access ODBC Driver"},
		DSNFormat:    "Driver={IBM i Access ODBC Driver};System=host;Uid=user;Pwd=pass",
	})
}
