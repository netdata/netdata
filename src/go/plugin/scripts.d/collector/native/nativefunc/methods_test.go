// SPDX-License-Identifier: GPL-3.0-or-later

package nativefunc

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validDefinition() Definition {
	return Definition{
		ID:             "items",
		Name:           "Items",
		Help:           "List items.",
		AcceptedParams: []string{"filter"},
		RequiredParams: []Parameter{
			{ID: "queue", Name: "Queue", Options: []Option{{ID: "mail", Name: "Mail", Default: true}}},
		},
	}
}

// edited returns one valid definition changed by edit.
func edited(edit func(*Definition)) []Definition {
	d := validDefinition()
	edit(&d)
	return []Definition{d}
}

func TestMethods(t *testing.T) {
	tests := map[string]struct {
		definitions []Definition
		want        []funcapi.FunctionConfig
		wantErr     bool
	}{
		"documented defaults": {
			definitions: []Definition{validDefinition()},
			want: []funcapi.FunctionConfig{{
				ID:             "items",
				Name:           "Items",
				Help:           "List items.",
				UpdateEvery:    10,
				ResponseType:   "table",
				RawRequest:     true,
				ManagedInfo:    true,
				AcceptedParams: []string{"filter"},
				RequiredParams: []funcapi.ParamConfig{{
					ID:        "queue",
					Name:      "Queue",
					Selection: funcapi.ParamSelect,
					Options:   []funcapi.ParamOption{{ID: "mail", Name: "Mail", Default: true}},
				}},
			}},
		},
		"all fields": {
			definitions: []Definition{{
				ID:             "items",
				Name:           "Items",
				Help:           "List items.",
				UpdateEvery:    37,
				ResponseType:   "custom",
				HasHistory:     true,
				AcceptedParams: []string{"queue"},
				RequiredParams: []Parameter{{
					ID:         "queue",
					Name:       "Queue",
					Help:       "Choose a queue.",
					Type:       "multiselect",
					UniqueView: true,
					Options:    []Option{{ID: "mail", Name: "Mail", Default: true, Disabled: true}},
				}},
			}},
			want: []funcapi.FunctionConfig{{
				ID:             "items",
				Name:           "Items",
				Help:           "List items.",
				UpdateEvery:    37,
				ResponseType:   "custom",
				RawRequest:     true,
				ManagedInfo:    true,
				HasHistory:     true,
				AcceptedParams: []string{"queue"},
				RequiredParams: []funcapi.ParamConfig{{
					ID:         "queue",
					Name:       "Queue",
					Help:       "Choose a queue.",
					Selection:  funcapi.ParamMultiSelect,
					UniqueView: true,
					Options:    []funcapi.ParamOption{{ID: "mail", Name: "Mail", Default: true, Disabled: true}},
				}},
			}},
		},
		"namespaced ID": {
			definitions: edited(func(d *Definition) { d.ID = "another:items" }),
			wantErr:     true,
		},
		"missing help": {
			definitions: edited(func(d *Definition) { d.Help = " " }),
			wantErr:     true,
		},
		"negative refresh": {
			definitions: edited(func(d *Definition) { d.UpdateEvery = -1 }),
			wantErr:     true,
		},
		"reserved accepted parameter": {
			definitions: edited(func(d *Definition) { d.AcceptedParams = []string{"__job"} }),
			wantErr:     true,
		},
		"duplicate accepted parameter": {
			definitions: edited(func(d *Definition) { d.AcceptedParams = []string{"x", "x"} }),
			wantErr:     true,
		},
		"duplicate method": {
			definitions: []Definition{validDefinition(), validDefinition()},
			wantErr:     true,
		},
		"reserved selector": {
			definitions: edited(func(d *Definition) { d.RequiredParams = []Parameter{{ID: "__job", Name: "Job"}} }),
			wantErr:     true,
		},
		"invalid selection type": {
			definitions: edited(func(d *Definition) {
				d.RequiredParams = []Parameter{{ID: "queue", Name: "Queue", Type: "text"}}
			}),
			wantErr: true,
		},
		"duplicate option": {
			definitions: edited(func(d *Definition) {
				d.RequiredParams = []Parameter{
					{ID: "q", Name: "Q", Options: []Option{{ID: "x", Name: "X"}, {ID: "x", Name: "X"}}},
				}
			}),
			wantErr: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			methods, err := Methods(tc.definitions)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, methods)
		})
	}
}
