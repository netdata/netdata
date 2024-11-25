// SPDX-License-Identifier: GPL-3.0-or-later

package testrandom

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (tr *TestRandom) validateConfig() error {
	if tr.Config.Charts.Num <= 0 && tr.Config.HiddenCharts.Num <= 0 {
		return errors.New("'charts->num' or `hidden_charts->num` must be > 0")
	}
	if tr.Config.Charts.Num > 0 && tr.Config.Charts.Dims <= 0 {
		return errors.New("'charts->dimensions' must be > 0")
	}
	if tr.Config.HiddenCharts.Num > 0 && tr.Config.HiddenCharts.Dims <= 0 {
		return errors.New("'hidden_charts->dimensions' must be > 0")
	}
	return nil
}

func (tr *TestRandom) initCharts() (*module.Charts, error) {
	charts := &module.Charts{}

	var ctx int
	v := calcContextEvery(tr.Config.Charts.Num, tr.Config.Charts.Contexts)
	for i := 0; i < tr.Config.Charts.Num; i++ {
		if i != 0 && v != 0 && ctx < (tr.Config.Charts.Contexts-1) && i%v == 0 {
			ctx++
		}
		chart := newChart(i, ctx, tr.Config.Charts.Labels, module.ChartType(tr.Config.Charts.Type))

		if err := charts.Add(chart); err != nil {
			return nil, err
		}
	}

	ctx = 0
	v = calcContextEvery(tr.Config.HiddenCharts.Num, tr.Config.HiddenCharts.Contexts)
	for i := 0; i < tr.Config.HiddenCharts.Num; i++ {
		if i != 0 && v != 0 && ctx < (tr.Config.HiddenCharts.Contexts-1) && i%v == 0 {
			ctx++
		}
		chart := newHiddenChart(i, ctx, tr.Config.HiddenCharts.Labels, module.ChartType(tr.Config.HiddenCharts.Type))

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
