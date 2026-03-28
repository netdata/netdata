// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	promselector "github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

type collectorRuntime struct {
	Profiles          []promprofiles.Profile
	compiledProfiles  []compiledProfile
	ChartTemplateYAML string
}

type compiledProfile struct {
	key     string
	profile promprofiles.Profile
	match   matcher.Matcher
	blocks  []compiledRelabelBlock
}

type compiledRelabelBlock struct {
	selector  promselector.Selector
	processor *relabel.Processor
}
