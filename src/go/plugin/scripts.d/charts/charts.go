// SPDX-License-Identifier: GPL-3.0-or-later

package charts

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/ids"
)

const (
	ctxPrefix = "nagios"
)

func ChartID(shard, jobName, suffix string) string {
	return fmt.Sprintf("nagios.%s.%s.%s", shard, ids.Sanitize(jobName), suffix)
}

func MetricKey(shard, jobName, suffix, dim string) string {
	return fmt.Sprintf("%s.%s", ChartID(shard, jobName, suffix), dim)
}

func jobChartBase(shard, jobName string) module.Chart {
	return module.Chart{
		Fam:  "nagios",
		Ctx:  fmt.Sprintf("%s.%s", ctxPrefix, shard),
		Type: module.Line,
		Labels: []module.Label{
			{Key: "nagios_job", Value: jobName, Source: module.LabelSourceConf},
			{Key: "nagios_shard", Value: shard, Source: module.LabelSourceConf},
		},
	}
}
