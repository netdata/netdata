// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticfunc

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
)

type method struct {
	rawfunc.Method
	serve func(h *Handler, ctx context.Context, args map[string]string, response map[string]any) *funcapi.FunctionResponse
}

var methods = []method{checksMethod, runsMethod, runMethod, artifactsMethod, artifactMethod}

func Declarations() []funcapi.FunctionConfig {
	out := make([]funcapi.FunctionConfig, 0, len(methods))
	for _, m := range methods {
		out = append(out, m.Declaration("synthetics"))
	}
	return out
}

// column declares a visible, sortable column. Timestamps are Unix microseconds.
func column[T any](name, tooltip string, kind funcapi.FieldType, units string, value func(T) any) rawfunc.Column[T] {
	col := rawfunc.Column[T]{
		Name:     name,
		Tooltip:  tooltip,
		Type:     kind,
		Units:    units,
		Visible:  true,
		Sortable: true,
		Value:    value,
	}
	if kind == funcapi.FieldTypeTimestamp {
		col.Transform = funcapi.FieldTransformDatetimeUsec
	}
	return col
}
