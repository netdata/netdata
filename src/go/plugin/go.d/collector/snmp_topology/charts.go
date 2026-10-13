// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import _ "embed"

//go:embed charts.yaml
var chartTemplateYAML string

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }
