// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"

type collectorRuntime struct {
	Profiles          []promprofiles.Profile
	ChartTemplateYAML string
}
