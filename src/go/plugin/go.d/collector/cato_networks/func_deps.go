// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cato_networks/catofunc"
)

type funcDepsAdapter struct {
	store *topologyStore
}

func (a funcDepsAdapter) CurrentTopology() (*topologyv1.Data, bool) {
	if a.store == nil {
		return nil, false
	}
	return a.store.CurrentTopology()
}

func catoMethods() []funcapi.MethodConfig {
	return catofunc.Methods(defaultUpdateEvery)
}

func catoFunctionHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}
