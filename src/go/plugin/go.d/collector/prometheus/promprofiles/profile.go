// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	promselector "github.com/netdata/netdata/go/plugins/pkg/prometheus/promselector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
	commonmodel "github.com/prometheus/common/model"
)

var validProfileName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

type Profile struct {
	Name              string                   `yaml:"name" json:"name"`
	Match             string                   `yaml:"match" json:"match"`
	MetricsRelabeling []MetricsRelabelingBlock `yaml:"metrics_relabeling,omitempty" json:"metrics_relabeling,omitempty"`
	Template          charttpl.Group           `yaml:"template" json:"template"`
}

type MetricsRelabelingBlock struct {
	Selector string           `yaml:"selector" json:"selector"`
	Rules    []relabel.Config `yaml:"rules" json:"rules"`
}

func (p Profile) Validate(path string) error {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return fmt.Errorf("%s.name: must not be empty", path)
	}
	if !validProfileName.MatchString(name) {
		return fmt.Errorf("%s.name: %q does not match ^[A-Za-z][A-Za-z0-9_]*$", path, p.Name)
	}
	if strings.TrimSpace(p.Match) == "" {
		return fmt.Errorf("%s.match: must not be empty", path)
	}
	if _, err := matcher.NewSimplePatternsMatcher(p.Match); err != nil {
		return fmt.Errorf("%s.match: %w", path, err)
	}

	for i, block := range p.MetricsRelabeling {
		if strings.TrimSpace(block.Selector) == "" {
			return fmt.Errorf("%s.metrics_relabeling[%d].selector: must not be empty", path, i)
		}
		if _, err := promselector.Parse(block.Selector); err != nil {
			return fmt.Errorf("%s.metrics_relabeling[%d].selector: %w", path, i, err)
		}
		if len(block.Rules) == 0 {
			return fmt.Errorf("%s.metrics_relabeling[%d].rules: must contain at least one rule", path, i)
		}
		if _, err := relabel.New(block.Rules); err != nil {
			return fmt.Errorf("%s.metrics_relabeling[%d].rules: %w", path, i, err)
		}
		for j, rule := range block.Rules {
			if err := validateCuratedRule(relabel.NormalizeConfig(rule)); err != nil {
				return fmt.Errorf("%s.metrics_relabeling[%d].rules[%d]: %w", path, i, j, err)
			}
		}
	}

	if !groupHasChart(p.Template) {
		return fmt.Errorf("%s.template: must contain at least one chart", path)
	}

	spec := charttpl.Spec{
		Version: charttpl.VersionV1,
		Groups:  []charttpl.Group{p.Template},
	}
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("%s.template: %w", path, err)
	}

	return nil
}

func validateCuratedRule(cfg relabel.Config) error {
	switch cfg.Action {
	case relabel.Replace:
		if containsTemplateVar(cfg.TargetLabel) {
			return fmt.Errorf("dynamic 'target_label' is not allowed in curated mode")
		}
		switch cfg.TargetLabel {
		case commonmodel.MetricNameLabel:
			if containsTemplateVar(cfg.Replacement) {
				return fmt.Errorf("capture-based or dynamic __name__ replacement is not allowed in curated mode")
			}
			if !cfg.NameScheme.IsValidMetricName(cfg.Replacement) {
				return fmt.Errorf("%q is not a valid constant final metric name in curated mode", cfg.Replacement)
			}
		case bucketLabel, quantileLabel:
			return fmt.Errorf("%q is immutable in curated mode", cfg.TargetLabel)
		}
	case relabel.Lowercase, relabel.Uppercase, relabel.HashMod:
		if isProtectedTargetLabel(cfg.TargetLabel) {
			return fmt.Errorf("%q must not be targeted by %s in curated mode", cfg.TargetLabel, cfg.Action)
		}
	case relabel.LabelMap:
		if containsTemplateVar(cfg.Replacement) {
			return fmt.Errorf("dynamic 'replacement' is not allowed for labelmap in curated mode")
		}
		if matchesProtectedLabel(cfg.Regex, commonmodel.MetricNameLabel) {
			return fmt.Errorf("__name__ must not be targeted by labelmap in curated mode")
		}
		if matchesProtectedLabel(cfg.Regex, bucketLabel) || matchesProtectedLabel(cfg.Regex, quantileLabel) {
			return fmt.Errorf("structural labels must not be targeted by labelmap in curated mode")
		}
		if isProtectedTargetLabel(cfg.Replacement) {
			return fmt.Errorf("%q must not be created by labelmap in curated mode", cfg.Replacement)
		}
	case relabel.LabelDrop:
		if matchesProtectedLabel(cfg.Regex, commonmodel.MetricNameLabel) {
			return fmt.Errorf("__name__ must not be targeted by labeldrop in curated mode")
		}
		if matchesProtectedLabel(cfg.Regex, bucketLabel) || matchesProtectedLabel(cfg.Regex, quantileLabel) {
			return fmt.Errorf("structural labels must not be targeted by labeldrop in curated mode")
		}
	case relabel.LabelKeep:
		return fmt.Errorf("labelkeep is not allowed in curated mode because it necessarily affects protected labels")
	}

	return nil
}

func containsTemplateVar(v string) bool {
	return strings.Contains(v, "$")
}

func isProtectedTargetLabel(name string) bool {
	return name == commonmodel.MetricNameLabel || name == bucketLabel || name == quantileLabel
}

func matchesProtectedLabel(re relabel.Regexp, label string) bool {
	return re.MatchString(label)
}

func groupHasChart(group charttpl.Group) bool {
	if len(group.Charts) > 0 {
		return true
	}
	for _, child := range group.Groups {
		if groupHasChart(child) {
			return true
		}
	}
	return false
}

const (
	bucketLabel   = "le"
	quantileLabel = "quantile"
)
