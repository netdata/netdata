// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import "github.com/netdata/netdata/go/plugins/pkg/selectorcore"

// Meta contains selector metadata needed by template compilers/engines.
type Meta struct {
	MetricNames          []string
	ConstrainedLabelKeys []string
}

// Compiled is a selector with stable metadata.
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

// ParseCompiled parses a selector expression and returns matcher + metadata.
func ParseCompiled(expr string) (Compiled, error) {
	compiled, err := selectorcore.ParseCompiled(expr)
	if err != nil {
		return nil, err
	}
	meta := compiled.Meta()
	out := compiledSelector{Selector: wrapCoreSelector(compiled)}
	out.meta = Meta{
		MetricNames:          append([]string(nil), meta.MetricNames...),
		ConstrainedLabelKeys: append([]string(nil), meta.ConstrainedLabelKeys...),
	}
	return out, nil
}
