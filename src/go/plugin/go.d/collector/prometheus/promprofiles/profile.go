// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

// Profile is a curated, exporter-specific chart profile. Match selects the
// profile by scraped metric names; Template is a standard v2 charttpl group
// rendered for the matched metrics. Identity is the profile file's basename
// (set by the loader), so the file itself carries no name field. The Template
// is validated exactly like any other v2 chart template, including its
// author-written metrics: visibility list.
type Profile struct {
	Name     string         `yaml:"-" json:"name"`
	Match    string         `yaml:"match" json:"match"`
	Template charttpl.Group `yaml:"template" json:"template"`
}

func (p *Profile) validate(path string) error {
	if strings.TrimSpace(p.Match) == "" {
		return fmt.Errorf("%s: 'match' must not be empty", path)
	}
	if _, err := matcher.NewSimplePatternsMatcher(p.Match); err != nil {
		return fmt.Errorf("%s: 'match': %w", path, err)
	}

	if !groupHasChart(p.Template) {
		return fmt.Errorf("%s: 'template' must contain at least one chart", path)
	}

	spec := charttpl.Spec{
		Version: charttpl.VersionV1,
		Groups:  []charttpl.Group{p.Template},
	}
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("%s: 'template': %w", path, err)
	}

	return nil
}

func groupHasChart(group charttpl.Group) bool {
	if len(group.Charts) > 0 {
		return true
	}
	return slices.ContainsFunc(group.Groups, groupHasChart)
}
