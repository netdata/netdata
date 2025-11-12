// SPDX-License-Identifier: GPL-3.0-or-later

package charts

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	ctxPrefix = "nagios"
)

func ChartIDFromParts(shard, chartKey, suffix string) string {
	return fmt.Sprintf("nagios.%s.%s.%s", shard, chartKey, suffix)
}

func MetricKeyFromParts(shard, chartKey, suffix, dim string) string {
	return fmt.Sprintf("%s.%s", ChartIDFromParts(shard, chartKey, suffix), dim)
}

func jobChartBase(meta JobIdentity) module.Chart {
	return module.Chart{
		Fam:    meta.ScriptKey,
		Ctx:    fmt.Sprintf("%s.%s", ctxPrefix, meta.ScriptKey),
		Type:   module.Line,
		Labels: meta.Labels(),
	}
}
