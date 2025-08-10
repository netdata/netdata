package ddprofiledefinition

import (
	"slices"
)

type VirtualMetricConfig struct {
	Name      string                      `yaml:"name"`
	Sources   []VirtualMetricSourceConfig `yaml:"sources"`
	ChartMeta ChartMeta                   `yaml:"chart_meta"`
}

func (vm VirtualMetricConfig) Clone() VirtualMetricConfig {
	return VirtualMetricConfig{
		Name:      vm.Name,
		Sources:   slices.Clone(vm.Sources),
		ChartMeta: vm.ChartMeta,
	}
}

type VirtualMetricSourceConfig struct {
	Metric string `yaml:"metric"`
	Table  string `yaml:"table"` // Required for now
}
