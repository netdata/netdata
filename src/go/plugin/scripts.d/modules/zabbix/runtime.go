// SPDX-License-Identifier: GPL-3.0-or-later

package zabbix

import (
	"context"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"
	pkgzabbix "github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/zabbix"
	zpre "github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/zabbixpreproc"
)

// runner is a thin wrapper that allows the module to delegate lifecycle control
// to the shared pkg/zabbix runtime while keeping module-specific wiring simple.
type runner struct {
	rt *pkgzabbix.Runtime
}

func newRunner(_ context.Context, log *logger.Logger, jobs []pkgzabbix.JobConfig, proc *zpre.Preprocessor, emitter runtime.ResultEmitter, vnodeLookup func(spec.JobSpec) runtime.VnodeInfo) (*runner, error) {
	rt, err := pkgzabbix.NewRuntime(jobs, proc, log, emitter, vnodeLookup)
	if err != nil {
		return nil, err
	}
	return &runner{rt: rt}, nil
}

func (r *runner) Collect() map[string]int64 {
	if r == nil || r.rt == nil {
		return nil
	}
	return r.rt.Collect()
}

func (r *runner) Stop() {
	if r == nil || r.rt == nil {
		return
	}
	r.rt.Stop()
}
