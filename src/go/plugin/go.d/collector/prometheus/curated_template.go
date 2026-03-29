// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

func buildCuratedChartTemplateYAML(profiles []promprofiles.Profile) (string, error) {
	spec := charttpl.Spec{
		Version:          charttpl.VersionV1,
		ContextNamespace: "prometheus",
		Engine: &charttpl.Engine{
			Autogen: &charttpl.EngineAutogen{Enabled: true},
		},
		Groups: make([]charttpl.Group, 0, len(profiles)),
	}

	if err := validateUniqueCuratedChartIDs(spec.ContextNamespace, profiles); err != nil {
		return "", err
	}

	for _, prof := range profiles {
		spec.Groups = append(spec.Groups, prof.Template)
	}

	if err := spec.Validate(); err != nil {
		return "", err
	}

	raw, err := yaml.Marshal(spec)
	if err != nil {
		return "", err
	}

	return string(raw), nil
}

func validateUniqueCuratedChartIDs(rootContextNamespace string, profiles []promprofiles.Profile) error {
	seenChartIDs := make(map[string]struct{})
	for _, prof := range profiles {
		if err := walkCuratedCharts(prof.Template, []string{rootContextNamespace}, func(chart charttpl.Chart, effectiveID string) error {
			if _, ok := seenChartIDs[effectiveID]; ok {
				return fmt.Errorf("chart id collision: %s", effectiveID)
			}
			seenChartIDs[effectiveID] = struct{}{}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func walkCuratedCharts(group charttpl.Group, inheritedContext []string, fn func(charttpl.Chart, string) error) error {
	contextParts := append(append([]string(nil), inheritedContext...), normalizeOptionalContextPart(group.ContextNamespace)...)
	for _, chart := range group.Charts {
		if err := fn(chart, effectiveCuratedChartID(chart, contextParts)); err != nil {
			return err
		}
	}
	for _, child := range group.Groups {
		if err := walkCuratedCharts(child, contextParts, fn); err != nil {
			return err
		}
	}
	return nil
}

func effectiveCuratedChartID(chart charttpl.Chart, inheritedContext []string) string {
	baseID := strings.TrimSpace(chart.ID)
	if baseID != "" {
		return baseID
	}

	contextParts := append(append([]string(nil), inheritedContext...), strings.TrimSpace(chart.Context))
	context := strings.Join(filterEmptyContextParts(contextParts), ".")
	return strings.ReplaceAll(context, ".", "_")
}

func normalizeOptionalContextPart(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return []string{value}
}

func filterEmptyContextParts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}
