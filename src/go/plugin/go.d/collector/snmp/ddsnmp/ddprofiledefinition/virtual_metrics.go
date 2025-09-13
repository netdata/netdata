package ddprofiledefinition

import (
	"slices"
)

type VirtualMetricConfig struct {
	Name      string                      `yaml:"name"`
	GroupBy   []string                    `yaml:"group_by"`
	Sources   []VirtualMetricSourceConfig `yaml:"sources"`
	ChartMeta ChartMeta                   `yaml:"chart_meta"`
}

func (vm VirtualMetricConfig) Clone() VirtualMetricConfig {
	return VirtualMetricConfig{
		Name:      vm.Name,
		GroupBy:   slices.Clone(vm.GroupBy),
		Sources:   slices.Clone(vm.Sources),
		ChartMeta: vm.ChartMeta,
	}
}

type VirtualMetricSourceConfig struct {
	Metric string `yaml:"metric"`
	Table  string `yaml:"table"` // Required for now
	As     string `yaml:"as"`    // dimension name for composite charts
}
