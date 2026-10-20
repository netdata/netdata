// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

type testMethodHandler struct{}

func (testMethodHandler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (testMethodHandler) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return &funcapi.FunctionResponse{Status: 200}
}

func (testMethodHandler) Cleanup(context.Context) {}

func TestFuncRouter_ConcurrentRegisterAndHandle(t *testing.T) {
	tests := map[string]struct {
		iterations int
	}{
		"registering a late handler does not race with function calls": {
			iterations: 200,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			router := newFuncRouter(newIfaceCache())
			handler := testMethodHandler{}
			ctx := context.Background()

			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				for i := 0; i < tc.iterations; i++ {
					router.registerHandler("dynamic", handler)
				}
			}()

			go func() {
				defer wg.Done()
				for i := 0; i < tc.iterations; i++ {
					_, _ = router.MethodParams(ctx, "dynamic")
					resp := router.Handle(ctx, "dynamic", nil)
					assert.NotNil(t, resp)
				}
			}()

			wg.Wait()
		})
	}
}
