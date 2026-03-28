// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	promselector "github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

var validProfileID = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type Profile struct {
	ID                string                   `yaml:"id" json:"id"`
	Name              string                   `yaml:"name,omitempty" json:"name,omitempty"`
	Match             string                   `yaml:"match" json:"match"`
	MetricsRelabeling []MetricsRelabelingBlock `yaml:"metrics_relabeling,omitempty" json:"metrics_relabeling,omitempty"`
	Template          charttpl.Group           `yaml:"template" json:"template"`
}

type MetricsRelabelingBlock struct {
	Selector string           `yaml:"selector" json:"selector"`
	Rules    []relabel.Config `yaml:"rules" json:"rules"`
}

func (p Profile) Validate(path string) error {
	id := strings.TrimSpace(p.ID)
	if id == "" {
		return fmt.Errorf("%s.id: must not be empty", path)
	}
	if !validProfileID.MatchString(id) {
		return fmt.Errorf("%s.id: %q does not match ^[a-z][a-z0-9_]*$", path, p.ID)
	}
	if p.Name != "" && strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("%s.name: must not be whitespace-only", path)
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
