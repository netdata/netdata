// SPDX-License-Identifier: GPL-3.0-or-later

package intelgpu

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioGPUFrequency = module.Priority + iota
	prioGPUPower
	prioGPUEngineBusy
)

var charts = module.Charts{
	intelGPUFrequencyChart.Copy(),
	intelGPUPowerGPUChart.Copy(),
}

var intelGPUFrequencyChart = module.Chart{
	ID:       "igpu_frequency",
	Title:    "Intel GPU frequency",
	Units:    "MHz",
	Fam:      "frequency",
	Ctx:      "intelgpu.frequency",
	Type:     module.Line,
	Priority: prioGPUFrequency,
	Dims: module.Dims{
		{ID: "frequency_actual", Name: "frequency", Div: precision},
	},
}

var intelGPUPowerGPUChart = module.Chart{
	ID:       "igpu_power_gpu",
	Title:    "Intel GPU power",
	Units:    "Watts",
	Fam:      "power",
	Ctx:      "intelgpu.power",
	Type:     module.Line,
	Priority: prioGPUPower,
	Dims: module.Dims{
		{ID: "power_gpu", Name: "gpu", Div: precision},
		{ID: "power_package", Name: "package", Div: precision},
	},
}

var intelGPUEngineBusyPercChartTmpl = module.Chart{
	ID:       "igpu_engine_%s_busy_percentage",
	Title:    "Intel GPU engine busy time percentage",
	Units:    "percentage",
	Fam:      "engines",
	Ctx:      "intelgpu.engine_busy_perc",
	Type:     module.Line,
	Priority: prioGPUEngineBusy,
	Dims: module.Dims{
		{ID: "engine_%s_busy", Name: "busy", Div: precision},
	},
}

func (c *Collector) addEngineCharts(engine string) {
	chart := intelGPUEngineBusyPercChartTmpl.Copy()

	s := strings.ToLower(engine)
	s = strings.ReplaceAll(s, "/", "_")

	chart.ID = fmt.Sprintf(chart.ID, s)
	chart.Labels = []module.Label{
		{Key: "engine_class", Value: engineClassName(engine)},
		{Key: "engine_instance", Value: engine},
	}
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, engine)
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}

func engineClassName(engine string) string {
	// https://gitlab.freedesktop.org/drm/igt-gpu-tools/-/blob/master/tools/intel_gpu_top.c#L431
	engines := []string{"Render/3D", "Blitter", "VideoEnhance", "Video", "Compute"}
	for _, name := range engines {
		if strings.HasPrefix(engine, name) {
			return name
		}
	}
	return "unknown"
}
