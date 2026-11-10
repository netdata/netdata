// SPDX-License-Identifier: GPL-3.0-or-later

package dem_test

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	rumfunctions "github.com/netdata/netdata/go/plugins/plugin/dem/rum/functions"
	rumhistory "github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	syntheticfunctions "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/functions"
	synthetichistory "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Retained grouping must work with no live collector, receiver or browser runtime.
func TestSiteAssociationHistoryFunctionsWithoutActiveJobs(t *testing.T) {
	ctx := context.Background()
	journal, err := demjournal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journal.Close()) })
	now := time.Now().Add(-time.Minute).UnixMicro()
	rum := rumhistory.NewStore(journal)
	runs := synthetichistory.NewStore(journal)
	for i, association := range []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", ""} {
		_, err := rum.AppendEvent(ctx, rumhistory.EventRecord{
			Site: "browser-traffic", VnodeGUID: association, SessionID: "same-session",
			ObservedUS: now + int64(i), Type: "pageview", Page: "/",
		})
		require.NoError(t, err)
		_, err = runs.AppendRun(ctx, "complete", synthetic.Run{
			ID:    []string{"run-storefront", "run-documentation", "run-unassigned"}[i],
			JobID: "journey:checkout", Kind: synthetic.Journey, Name: "checkout", VnodeGUID: association,
			StartedUS: now + int64(i), CompletedUS: now + int64(i) + 1, Outcome: synthetic.Success,
		})
		require.NoError(t, err)
	}
	require.NoError(t, runs.Sync(ctx))
	components := dem.New(dem.Dependencies{History: journal}, dem.DefaultConfig())
	rumHandler := components.Functions[0].NewHandlerWithContext(funcapi.ProcessFunctionContext{}).(*rumfunctions.Handler)
	syntheticHandler := components.Functions[1].NewHandlerWithContext(funcapi.ProcessFunctionContext{}).(*syntheticfunctions.Handler)
	for name, tc := range map[string]struct {
		method string
		handle func(context.Context, funcapi.RawMethodRequest) *funcapi.FunctionResponse
	}{
		"RUM":       {"rum-sessions", rumHandler.HandleRaw},
		"synthetic": {"synthetics-runs", syntheticHandler.HandleRaw},
	} {
		t.Run(name, func(t *testing.T) {
			for _, association := range []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"} {
				response := tc.handle(ctx, funcapi.RawMethodRequest{
					Method: tc.method, Args: []string{"vnode_guid:" + association}, Permissions: "0x7b",
				})
				require.NotNil(t, response.RawResponse)
				rows := response.RawResponse["data"].([][]any)
				require.Len(t, rows, 1)
				assert.Equal(
					t,
					association,
					rows[0][response.RawResponse["columns"].(map[string]any)["vnode_guid"].(map[string]any)["index"].(int)],
				)
			}
			response := tc.handle(ctx, funcapi.RawMethodRequest{
				Method:      tc.method,
				Permissions: "0x7b",
			})
			require.NotNil(t, response.RawResponse)
			rows := response.RawResponse["data"].([][]any)
			require.Len(t, rows, 3)
			var groups []any
			for _, row := range rows {
				index := response.RawResponse["columns"].(map[string]any)["vnode_guid"].(map[string]any)["index"].(int)
				groups = append(groups, row[index])
			}
			assert.ElementsMatch(t, []any{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", ""}, groups)
		})
	}
}
