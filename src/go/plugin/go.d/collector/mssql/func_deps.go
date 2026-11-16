// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/mssql/mssqlfunc"
)

type functionDeps struct{ collector *Collector }

var _ mssqlfunc.Deps = functionDeps{}

func (d functionDeps) DB() mssqlfunc.Queryer {
	if d.collector.functionDB == nil {
		return nil
	}
	return d.collector.functionDB
}

func (d functionDeps) ServerInfo() mssqlfunc.ServerInfo {
	_, major, edition, _ := d.collector.serverProperties()
	return mssqlfunc.ServerInfo{
		MajorVersion:     major,
		AzureSQLDatabase: edition == engineEditionAzureSQLDatabase,
	}
}

func (d functionDeps) EnsureServerInfo(ctx context.Context) error {
	_, err := d.collector.ensureEngineEdition(ctx)
	return err
}

func mssqlFunctionHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}
