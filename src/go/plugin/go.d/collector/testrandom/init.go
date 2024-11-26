// SPDX-License-Identifier: GPL-3.0-or-later

package testrandom

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (c *Collector) validateConfig() error {
	if c.Config.Charts.Num <= 0 && c.Config.HiddenCharts.Num <= 0 {
		return errors.New("'charts->num' or `hidden_charts->num` must be > 0")
	}
	if c.Config.Charts.Num > 0 && c.Config.Charts.Dims <= 0 {
		return errors.New("'charts->dimensions' must be > 0")
	}
	if c.Config.HiddenCharts.Num > 0 && c.Config.HiddenCharts.Dims <= 0 {
		return errors.New("'hidden_charts->dimensions' must be > 0")
	}
	return nil
}

func (c *Collector) initCharts() (*module.Charts, error) {
	charts := &module.Charts{}

	var ctx int
	v := calcContextEvery(c.Config.Charts.Num, c.Config.Charts.Contexts)
	for i := 0; i < c.Config.Charts.Num; i++ {
		if i != 0 && v != 0 && ctx < (c.Config.Charts.Contexts-1) && i%v == 0 {
			ctx++
		}
		chart := newChart(i, ctx, c.Config.Charts.Labels, module.ChartType(c.Config.Charts.Type))

		if err := charts.Add(chart); err != nil {
			return nil, err
		}
	}

	ctx = 0
	v = calcContextEvery(c.Config.HiddenCharts.Num, c.Config.HiddenCharts.Contexts)
	for i := 0; i < c.Config.HiddenCharts.Num; i++ {
		if i != 0 && v != 0 && ctx < (c.Config.HiddenCharts.Contexts-1) && i%v == 0 {
			ctx++
		}
		chart := newHiddenChart(i, ctx, c.Config.HiddenCharts.Labels, module.ChartType(c.Config.HiddenCharts.Type))

		if err := charts.Add(chart); err != nil {
			return nil, err
		}
	}

	return charts, nil
}

func calcContextEvery(charts, contexts int) int {
	if contexts <= 1 {
		return 0
	}
	if contexts > charts {
		return 1
	}
	return charts / contexts
}
