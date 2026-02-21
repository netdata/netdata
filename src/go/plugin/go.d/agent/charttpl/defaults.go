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
		applyGroupDefaults(&spec.Groups[i])
	}
}

func applyGroupDefaults(group *Group) {
	for i := range group.Charts {
		if group.Charts[i].Type == "" {
			group.Charts[i].Type = defaultChartType
		}
	}
	for i := range group.Groups {
		applyGroupDefaults(&group.Groups[i])
	}
}
