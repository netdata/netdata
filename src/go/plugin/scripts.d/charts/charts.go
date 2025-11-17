// SPDX-License-Identifier: GPL-3.0-or-later

package charts

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	ctxPrefix = "nagios"
)

func SchedulerChartID(scheduler, suffix string) string {
	return fmt.Sprintf("%s.scheduler.%s", scheduler, suffix)
}

func SchedulerMetricKey(scheduler, suffix, dim string) string {
	return fmt.Sprintf("%s.%s", SchedulerChartID(scheduler, suffix), dim)
}

func telemetryChartBase(meta JobIdentity, metric string) module.Chart {
	return module.Chart{
		Fam:          "jobs",
		Ctx:          fmt.Sprintf("%s.jobs.%s", ctxPrefix, metric),
		Type:         module.Line,
		TypeOverride: ctxPrefix,
		Labels:       meta.Labels(),
	}
}

func perfdataChartBase(meta JobIdentity) module.Chart {
	return module.Chart{
		Fam:          meta.ScriptKey,
		Ctx:          fmt.Sprintf("%s.%s", ctxPrefix, meta.ScriptKey),
		Type:         module.Line,
		TypeOverride: ctxPrefix,
		Labels:       meta.Labels(),
	}
}
