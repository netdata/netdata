// SPDX-License-Identifier: GPL-3.0-or-later

package nativefunc

import (
	"context"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeclarations(t *testing.T) {
	good := Definition{
		ID:             "items",
		Name:           "Items",
		Help:           "List items.",
		AcceptedParams: []string{"filter"},
		RequiredParams: []Parameter{
			{ID: "queue", Name: "Queue", Options: []Option{{ID: "mail", Name: "Mail", Default: true}}},
		},
	}
	methods, err := Declarations([]Definition{good})
	require.NoError(t, err)
	require.Len(t, methods, 1)
	assert.True(t, methods[0].RawRequest)
	assert.True(t, methods[0].ManagedInfo)
	assert.Equal(t, 10, methods[0].UpdateEvery)
	assert.Equal(t, "table", methods[0].ResponseType)
	for name, edit := range map[string]func(*Definition){
		"namespace":          func(d *Definition) { d.ID = "another:items" },
		"missing help":       func(d *Definition) { d.Help = " " },
		"negative refresh":   func(d *Definition) { d.UpdateEvery = -1 },
		"reserved accepted":  func(d *Definition) { d.AcceptedParams = []string{"__job"} },
		"duplicate accepted": func(d *Definition) { d.AcceptedParams = []string{"x", "x"} },
		"reserved selector":  func(d *Definition) { d.RequiredParams = []Parameter{{ID: "__job", Name: "Job"}} },
		"invalid selector":   func(d *Definition) { d.RequiredParams = []Parameter{{ID: "queue", Name: "Queue", Type: "text"}} },
		"duplicate option": func(d *Definition) {
			d.RequiredParams = []Parameter{{ID: "q", Name: "Q", Options: []Option{{ID: "x", Name: "X"}, {ID: "x", Name: "X"}}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			d := good
			edit(&d)
			_, err := Declarations([]Definition{d})
			require.Error(t, err)
		})
	}
	_, err = Declarations([]Definition{good, good})
	require.Error(t, err)
}

type stubDeps struct {
	err   error
	calls int
}

func (d *stubDeps) ExecuteFunction(context.Context, funcapi.RawMethodRequest) (*funcapi.FunctionResponse, error) {
	d.calls++
	return &funcapi.FunctionResponse{
		Status: 200,
	}, d.err
}
func TestRouter(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		status int
	}{"ok": {nil, 200}, "timeout": {context.DeadlineExceeded, 504}, "cancel": {context.Canceled, 499}, "failure": {errors.New("secret peer bytes"), 502}} {
		t.Run(name, func(t *testing.T) {
			d := &stubDeps{
				err: tc.err,
			}
			h := New(d, []funcapi.FunctionConfig{{ID: "items"}}).(funcapi.RawMethodHandler)
			r := h.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method: "items",
			})
			assert.Equal(t, tc.status, r.Status)
			assert.NotContains(t, r.Message, "secret")
			assert.Equal(t, 1, d.calls)
			r = h.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method: "other",
			})
			assert.Equal(t, 404, r.Status)
			assert.Equal(t, 1, d.calls)
		})
	}
}
