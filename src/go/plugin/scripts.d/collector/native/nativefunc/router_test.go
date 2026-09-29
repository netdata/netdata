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
		method     string
		err        error
		wantStatus int
		wantCalls  int
	}{
		"success":           {method: "items", wantStatus: 200, wantCalls: 1},
		"timeout":           {method: "items", err: context.DeadlineExceeded, wantStatus: 504, wantCalls: 1},
		"canceled":          {method: "items", err: context.Canceled, wantStatus: 499, wantCalls: 1},
		"failure":           {method: "items", err: errors.New("secret peer bytes"), wantStatus: 502, wantCalls: 1},
		"undeclared method": {method: "other", wantStatus: 404, wantCalls: 0},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			deps := &stubDeps{
				err: tc.err,
			}
			router := NewRouter(deps, []funcapi.FunctionConfig{{ID: "items"}}).(funcapi.RawMethodHandler)
			result := router.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method: tc.method,
			})
			assert.Equal(t, tc.wantStatus, result.Status)
			assert.NotContains(t, result.Message, "secret", "messages never echo execution errors")
			assert.Equal(t, tc.wantCalls, deps.calls)
		})
	}
}
