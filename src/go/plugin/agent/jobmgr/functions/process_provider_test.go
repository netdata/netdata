// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/require"
)

func TestContextProviderWithoutProductionComposition(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{true: "no declarations", false: "declared"}[empty], func(t *testing.T) {
			calls := 0
			handler := &controllerTestHandler{}
			provider := funcapi.ProcessFunctionProvider{
				ID: "history",
				Functions: func() []funcapi.FunctionConfig {
					if empty {
						return nil
					}
					return []funcapi.FunctionConfig{{ID: "read"}}
				},
				NewHandlerWithContext: func(ctx funcapi.ProcessFunctionContext) funcapi.MethodHandler {
					calls++
					require.Nil(t, ctx.LookupVnodeGUID)
					return handler
				},
			}
			require.NoError(t, ValidateProcessProviders([]funcapi.ProcessFunctionProvider{provider}))
			plan, err := buildProcessProviderPlan(provider)
			require.NoError(t, err)
			plan.agentBundle.retire()
			require.NoError(t, plan.agentBundle.wait(context.Background()))
			if empty {
				require.Zero(t, calls)
				require.Zero(t, handler.cleanups)
			} else {
				require.Equal(t, 1, calls)
				require.Equal(t, 1, handler.cleanups)
			}
		})
	}
}

func TestContextProviderFactoryPanicIsContained(t *testing.T) {
	_, err := buildProcessProviderPlan(funcapi.ProcessFunctionProvider{
		ID:                    "history",
		Functions:             func() []funcapi.FunctionConfig { return []funcapi.FunctionConfig{{ID: "read"}} },
		NewHandlerWithContext: func(funcapi.ProcessFunctionContext) funcapi.MethodHandler { panic("fixture") },
	})
	require.ErrorContains(t, err, "panic")
}
