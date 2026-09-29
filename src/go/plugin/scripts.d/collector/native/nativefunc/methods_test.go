// SPDX-License-Identifier: GPL-3.0-or-later

package nativefunc

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMethods(t *testing.T) {
	valid := func() Definition {
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
	tests := map[string]struct {
		edit    func(*[]Definition)
		want    []funcapi.FunctionConfig
		wantErr bool
	}{
		"defaults": {
			edit: func(*[]Definition) {},
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
			edit: func(d *[]Definition) {
				(*d)[0] = Definition{
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
				}
			},
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
		"namespaced ID":    {edit: func(d *[]Definition) { (*d)[0].ID = "another:items" }, wantErr: true},
		"missing help":     {edit: func(d *[]Definition) { (*d)[0].Help = " " }, wantErr: true},
		"negative refresh": {edit: func(d *[]Definition) { (*d)[0].UpdateEvery = -1 }, wantErr: true},
		"reserved accepted": {
			edit:    func(d *[]Definition) { (*d)[0].AcceptedParams = []string{jobSelector} },
			wantErr: true,
		},
		"duplicate accepted": {
			edit:    func(d *[]Definition) { (*d)[0].AcceptedParams = []string{"x", "x"} },
			wantErr: true,
		},
		"duplicate method": {edit: func(d *[]Definition) { *d = append(*d, (*d)[0]) }, wantErr: true},
		"reserved selector": {
			edit:    func(d *[]Definition) { (*d)[0].RequiredParams = []Parameter{{ID: jobSelector, Name: "Job"}} },
			wantErr: true,
		},
		"invalid selection type": {
			edit: func(d *[]Definition) {
				(*d)[0].RequiredParams = []Parameter{{ID: "queue", Name: "Queue", Type: "text"}}
			},
			wantErr: true,
		},
		"duplicate option": {
			edit: func(d *[]Definition) {
				(*d)[0].RequiredParams = []Parameter{
					{ID: "q", Name: "Q", Options: []Option{{ID: "x", Name: "X"}, {ID: "x", Name: "X"}}},
				}
			},
			wantErr: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			definitions := []Definition{valid()}
			tc.edit(&definitions)
			methods, err := Methods(definitions)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, methods)
		})
	}
}
