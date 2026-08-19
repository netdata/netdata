// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import "fmt"

const (
	OverlayProviderNetdataMetrics  = "netdata.metrics"
	OverlayProviderNetdataFunction = "netdata.function"
	OverlayProviderExternal        = "external"

	OverlayMergeRefsAppend = "append"
	OverlayMergeRefsSet    = "set"

	OverlayMergeValuesSum  = "sum"
	OverlayMergeValuesMin  = "min"
	OverlayMergeValuesMax  = "max"
	OverlayMergeValuesAvg  = "avg"
	OverlayMergeValuesLast = "last"
	OverlayMergeValuesNone = "none"

	OverlayRefsTemplateColumn = "template"
	OverlayRefsActorColumn    = "actor"
	OverlayRefsLinkColumn     = "link"
)

type OverlayTemplateOption func(*OverlayTemplate)

func NewOverlayTemplate(provider string, merge OverlayMerge, opts ...OverlayTemplateOption) OverlayTemplate {
	template := OverlayTemplate{
		Provider: provider,
		Merge:    merge,
	}
	for _, opt := range opts {
		opt(&template)
	}
	return template
}

func WithOverlayContexts(contexts ...string) OverlayTemplateOption {
	return func(template *OverlayTemplate) {
		template.Contexts = append([]string(nil), contexts...)
	}
}

func WithOverlayDimensions(dimensions ...string) OverlayTemplateOption {
	return func(template *OverlayTemplate) {
		template.Dimensions = append([]string(nil), dimensions...)
	}
}

func WithOverlaySelectorParams(params ...string) OverlayTemplateOption {
	return func(template *OverlayTemplate) {
		template.SelectorParams = append([]string(nil), params...)
	}
}

func NewOverlayMerge(refs, values string) OverlayMerge {
	return OverlayMerge{
		Refs:   refs,
		Values: values,
	}
}

type OverlayRefsBuilder struct {
	strings       *StringDictionary
	selectorNames []string
	builder       *TableBuilder
	err           error
}

func NewActorOverlayRefsBuilder(strings *StringDictionary, selectorNames ...string) *OverlayRefsBuilder {
	return newOverlayRefsBuilder(strings, NewColumn(OverlayRefsActorColumn, "actor_ref", WithRole("reference")), selectorNames...)
}

func NewLinkOverlayRefsBuilder(strings *StringDictionary, selectorNames ...string) *OverlayRefsBuilder {
	return newOverlayRefsBuilder(strings, NewColumn(OverlayRefsLinkColumn, "link_ref", WithRole("reference")), selectorNames...)
}

func newOverlayRefsBuilder(strings *StringDictionary, ownerColumn Column, selectorNames ...string) *OverlayRefsBuilder {
	columns := []Column{
		NewColumn(OverlayRefsTemplateColumn, "string_ref", WithDictionary("strings")),
		ownerColumn,
	}
	for _, name := range selectorNames {
		columns = append(columns, NewColumn(name, "string_ref", WithDictionary("strings")))
	}

	return &OverlayRefsBuilder{
		strings:       strings,
		selectorNames: append([]string(nil), selectorNames...),
		builder:       NewTableBuilder(columns...),
	}
}

func (b *OverlayRefsBuilder) Add(template string, owner int, selectorValues ...string) int {
	row := b.builder.Rows()
	if b.err != nil {
		return row
	}
	if b.strings == nil {
		b.err = fmt.Errorf("overlay refs builder requires string dictionary")
		return row
	}
	if len(selectorValues) != len(b.selectorNames) {
		b.err = fmt.Errorf("overlay ref has %d selector values for %d selector params", len(selectorValues), len(b.selectorNames))
		return row
	}

	values := make([]any, 0, len(selectorValues)+2)
	values = append(values, b.strings.Ref(template), owner)
	for _, value := range selectorValues {
		values = append(values, b.strings.Ref(value))
	}
	return b.builder.Add(values...)
}

func (b *OverlayRefsBuilder) Rows() int {
	return b.builder.Rows()
}

func (b *OverlayRefsBuilder) OverlayRefs() (*OverlayRefs, error) {
	if b.err != nil {
		return nil, b.err
	}
	if b.Rows() == 0 {
		return nil, nil
	}
	table, err := b.builder.Table()
	if err != nil {
		return nil, err
	}
	return &OverlayRefs{Refs: &table}, nil
}
