// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	_ "embed"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

const (
	contextNamespace = "statsd"
	// defaultChartExpiry, in successful cycles, applies to generic charts and to
	// omitted profile chart and dimension expiry.
	defaultChartExpiry = 5
	// diagnosticsEntryID cannot collide with a profile entry: profile names start with a letter.
	diagnosticsEntryID = "_receiver"
)

//go:embed charts.yaml
var chartsYAML []byte

// diagnosticsTemplate decodes the embedded receiver diagnostics template once.
// NewTemplateSet deep-copies entries, so jobs may share the decoded groups.
var diagnosticsTemplate = sync.OnceValues(func() (chartengine.TemplateEntry, error) {
	spec, err := charttpl.DecodeYAML(chartsYAML)
	if err != nil {
		return chartengine.TemplateEntry{}, err
	}
	return chartengine.TemplateEntry{
		ID:               diagnosticsEntryID,
		ContextNamespace: spec.ContextNamespace,
		Groups:           spec.Groups,
	}, nil
})

// enginePolicy is fixed for a running job; every snapshot uses the same value.
func enginePolicy() chartengine.EnginePolicy {
	return chartengine.EnginePolicy{
		Autogen: &chartengine.AutogenPolicy{
			Enabled:                  true,
			ExpireAfterSuccessCycles: defaultChartExpiry,
		},
	}
}

// templateSet composes the diagnostics entry with the active profile entries,
// in configured order.
func (c *Collector) templateSet(active []bool) (*chartengine.TemplateSet, error) {
	diagnostics, err := diagnosticsTemplate()
	if err != nil {
		return nil, err
	}
	entries := []chartengine.TemplateEntry{diagnostics}
	for i, p := range c.profiles {
		if p.groups != nil && i < len(active) && active[i] {
			entries = append(entries, p.entry())
		}
	}
	return chartengine.NewTemplateSet(chartengine.TemplateSetSpec{
		Entries:                  entries,
		Policy:                   enginePolicy(),
		FallbackContextNamespace: contextNamespace,
	})
}
