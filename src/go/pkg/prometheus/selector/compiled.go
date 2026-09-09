// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import "github.com/netdata/netdata/go/plugins/pkg/selectorcore"

// Meta contains selector metadata used to build exact-name indexes without
// changing selector matching semantics.
type Meta struct {
	MetricNames          []string
	ConstrainedLabelKeys []string
}

// Compiled is a Prometheus-label selector with stable metadata.
type Compiled interface {
	Selector
	Meta() Meta
}

type compiledSelector struct {
	Selector
	meta Meta
}

func (c compiledSelector) Meta() Meta {
	return Meta{
		MetricNames:          append([]string(nil), c.meta.MetricNames...),
		ConstrainedLabelKeys: append([]string(nil), c.meta.ConstrainedLabelKeys...),
	}
}

// ParseCompiled parses expr through the production selector grammar and
// returns its matcher plus conservative exact-name/index metadata.
func ParseCompiled(expr string) (Compiled, error) {
	compiled, err := selectorcore.ParseCompiled(expr)
	if err != nil {
		return nil, err
	}
	meta := compiled.Meta()
	return compiledSelector{
		Selector: wrapCoreSelector(compiled),
		meta: Meta{
			MetricNames:          append([]string(nil), meta.MetricNames...),
			ConstrainedLabelKeys: append([]string(nil), meta.ConstrainedLabelKeys...),
		},
	}, nil
}
