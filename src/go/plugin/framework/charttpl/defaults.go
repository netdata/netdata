// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

const (
	defaultChartType = "line"
)

// applyDefaults mutates parsed template with phase-1 defaults.
func applyDefaults(spec *Spec) {
	if spec == nil {
		return
	}
	if spec.Version == "" {
		spec.Version = VersionV1
	}
	for i := range spec.Groups {
		applyGroupDefaults(&spec.Groups[i], nil)
	}
}

func applyGroupDefaults(group *Group, inherited *ChartDefaults) {
	effective := inheritChartDefaults(inherited, group.ChartDefaults)
	for i := range group.Charts {
		applyChartDefaults(&group.Charts[i], effective)
		if group.Charts[i].Type == "" {
			group.Charts[i].Type = defaultChartType
		}
	}
	for i := range group.Groups {
		applyGroupDefaults(&group.Groups[i], effective)
	}
}

func applyChartDefaults(chart *Chart, defaults *ChartDefaults) {
	if chart == nil || defaults == nil {
		return
	}
	if chart.LabelPromoted == nil && defaults.LabelPromoted != nil {
		chart.LabelPromoted = append([]string(nil), defaults.LabelPromoted...)
	}
	if chart.Instances == nil && defaults.Instances != nil {
		chart.Instances = cloneInstances(defaults.Instances)
	}
}

func inheritChartDefaults(parent, own *ChartDefaults) *ChartDefaults {
	if parent == nil && own == nil {
		return nil
	}

	out := &ChartDefaults{}
	if parent != nil {
		if parent.LabelPromoted != nil {
			out.LabelPromoted = append([]string(nil), parent.LabelPromoted...)
		}
		if parent.Instances != nil {
			out.Instances = cloneInstances(parent.Instances)
		}
	}
	if own != nil {
		if own.LabelPromoted != nil {
			out.LabelPromoted = append([]string(nil), own.LabelPromoted...)
		}
		if own.Instances != nil {
			out.Instances = cloneInstances(own.Instances)
		}
	}
	if out.LabelPromoted == nil && out.Instances == nil {
		return nil
	}
	return out
}

func cloneInstances(in *Instances) *Instances {
	if in == nil {
		return nil
	}
	return &Instances{
		ByLabels: append([]string(nil), in.ByLabels...),
	}
}
