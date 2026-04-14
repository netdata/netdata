package ddprofiledefinition

import (
	"slices"
)

type VirtualMetricConfig struct {
	Name         string                                  `yaml:"name"`
	PerRow       bool                                    `yaml:"per_row"`
	GroupBy      []string                                `yaml:"group_by"`
	EmitTags     []VirtualMetricEmitTagConfig            `yaml:"emit_tags"`
	Sources      []VirtualMetricSourceConfig             `yaml:"sources"`
	Alternatives []VirtualMetricAlternativeSourcesConfig `yaml:"alternatives"`
	ChartMeta    ChartMeta                               `yaml:"chart_meta"`
}

func (vm VirtualMetricConfig) Clone() VirtualMetricConfig {
	alts := make([]VirtualMetricAlternativeSourcesConfig, len(vm.Alternatives))
	for i, alt := range vm.Alternatives {
		alts[i] = VirtualMetricAlternativeSourcesConfig{
			Sources: slices.Clone(alt.Sources),
		}
	}

	return VirtualMetricConfig{
		Name:         vm.Name,
		PerRow:       vm.PerRow,
		GroupBy:      slices.Clone(vm.GroupBy),
		EmitTags:     slices.Clone(vm.EmitTags),
		Sources:      slices.Clone(vm.Sources),
		Alternatives: alts,
		ChartMeta:    vm.ChartMeta,
	}
}

type (
	VirtualMetricEmitTagConfig struct {
		Tag  string `yaml:"tag"`
		From string `yaml:"from"`
	}
	VirtualMetricSourceConfig struct {
		Metric string `yaml:"metric"`
		Table  string `yaml:"table"`
		As     string `yaml:"as"`  // dimension name for composite charts
		Dim    string `yaml:"dim"` // optional MultiValue dimension selector for source metrics
	}
	VirtualMetricAlternativeSourcesConfig struct {
		Sources []VirtualMetricSourceConfig `yaml:"sources"`
	}
)
