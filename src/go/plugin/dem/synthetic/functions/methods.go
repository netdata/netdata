// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

type method struct {
	id, title, help, sort string
	params                []string
	history               bool
	columns               map[string]any
}

type column struct {
	id, title string
	kind      funcapi.FieldType
	units     string
}

func columns(fields ...column) map[string]any {
	out := make(map[string]any, len(fields))
	for i, f := range fields {
		transform := funcapi.FieldTransformNone
		if f.kind == funcapi.FieldTypeTimestamp {
			transform = funcapi.FieldTransformDatetimeUsec
			if strings.HasSuffix(f.id, "_ms") {
				transform = funcapi.FieldTransformDatetime
			}
		}
		out[f.id] = (funcapi.Column{
			ValueOptions: funcapi.ValueOptions{
				Transform: transform,
			},
			Index:         i,
			Name:          f.title,
			Type:          f.kind,
			Units:         f.units,
			Visualization: funcapi.FieldVisualValue,
			Visible:       true,
			Sortable:      true,
		}).BuildColumn()
	}
	return out
}

var methods = []method{checksMethod, runsMethod, runMethod, artifactMethod}

func Declarations() []funcapi.FunctionConfig {
	out := make([]funcapi.FunctionConfig, 0, len(methods))
	for _, m := range methods {
		out = append(
			out,
			funcapi.FunctionConfig{
				ID:             m.id,
				FunctionName:   m.id,
				Name:           m.title,
				UpdateEvery:    10,
				Help:           m.help,
				Tags:           "synthetics",
				RawRequest:     true,
				ManagedInfo:    true,
				HasHistory:     m.history,
				AcceptedParams: append([]string(nil), m.params...),
			},
		)
	}
	return out
}
