// SPDX-License-Identifier: GPL-3.0-or-later

package nativefunc

import (
	"context"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

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

func TestRouter_HandleRaw(t *testing.T) {
	tests := map[string]struct {
		err        error
		wantStatus int
	}{
		"success":  {wantStatus: 200},
		"timeout":  {err: context.DeadlineExceeded, wantStatus: 504},
		"canceled": {err: context.Canceled, wantStatus: 499},
		"failure":  {err: errors.New("secret peer bytes"), wantStatus: 502},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			deps := &stubDeps{
				err: tc.err,
			}
			router := NewRouter(deps, []funcapi.FunctionConfig{{ID: "items"}}).(funcapi.RawMethodHandler)

			result := router.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method: "items",
			})
			assert.Equal(t, tc.wantStatus, result.Status)
			assert.NotContains(t, result.Message, "secret")
			assert.Equal(t, 1, deps.calls)

			result = router.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method: "other",
			})
			assert.Equal(t, 404, result.Status)
			assert.Equal(t, 1, deps.calls, "undeclared methods must not reach deps")
		})
	}
}
