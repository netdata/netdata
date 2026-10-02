// SPDX-License-Identifier: GPL-3.0-or-later

package mssqlfunc

import (
	"context"

	"github.com/netdata/netdata/go/plugins/logger"
)

const engineEditionAzureSQLDatabase = 5

type testDeps struct {
	db   Queryer
	info ServerInfo
}

func (d *testDeps) DB() Queryer                            { return d.db }
func (d *testDeps) ServerInfo() ServerInfo                 { return d.info }
func (d *testDeps) EnsureServerInfo(context.Context) error { return nil }

func newTestRouter(db Queryer) *router {
	return newRouter(&testDeps{
		db: db,
	}, logger.New(), FunctionsConfig{})
}
