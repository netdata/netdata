// SPDX-License-Identifier: GPL-3.0-or-later

package rumfunc_test

import (
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journalstore"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/rumfunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiveCursorReadsOnlyNewerObservations(t *testing.T) {
	declarations := rumfunc.Declarations()
	decl := declarations[slices.IndexFunc(declarations, func(d funcapi.FunctionConfig) bool {
		return d.ID == "rum-live"
	})]
	for name, request := range map[string]func(cursor string) funcapi.RawMethodRequest{
		"arguments": func(cursor string) funcapi.RawMethodRequest {
			args := []string{"site:shop"}
			if cursor != "" {
				args = append(args, "cursor:"+cursor)
			}
			return funcapi.RawMethodRequest{
				Method:      decl.ID,
				Args:        args,
				Permissions: "0x1b",
			}
		},
		// The table posts its time window only into accepted parameters, so it cannot reach the cursor.
		"cloud table body": func(cursor string) funcapi.RawMethodRequest {
			fields := map[string]any{"site": "shop"}
			if cursor != "" {
				fields["cursor"] = cursor
			}
			return funcapi.RawMethodRequest{
				Method:      decl.ID,
				Payload:     cloudTableBody(t, decl, fields),
				Permissions: "0x1b",
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			journalStore, err := journalstore.Open(t.Context(), "")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
			hub := rumregistry.New()
			a, _ := addSite(t, hub, "shop", "first")
			handler := rumfunc.New(query.New(hub, history.NewStore(journalStore)), nil)
			pages := func(response map[string]any) []any {
				index := response["columns"].(map[string]any)["page"].(map[string]any)["index"].(int)
				var out []any
				for _, row := range response["data"].([][]any) {
					out = append(out, row[index])
				}
				return out
			}

			page(a, "shop", "/one")
			first := handler.HandleRaw(t.Context(), request("")).RawResponse
			require.NotNil(t, first)
			assert.Equal(t, []any{"/one"}, pages(first))
			next, ok := first["next"].(string)
			require.True(t, ok)
			require.NotEmpty(t, next)

			page(a, "shop", "/two")
			second := handler.HandleRaw(t.Context(), request(next)).RawResponse
			require.NotNil(t, second)
			assert.Equal(t, []any{"/two"}, pages(second))
		})
	}
}

func TestLiveRejectsTheFormerCursorName(t *testing.T) {
	journalStore, err := journalstore.Open(t.Context(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	hub := rumregistry.New()
	a, _ := addSite(t, hub, "shop", "first")
	handler := rumfunc.New(query.New(hub, history.NewStore(journalStore)), nil)
	page(a, "shop", "/one")
	first := handler.HandleRaw(t.Context(), funcapi.RawMethodRequest{
		Method:      "rum-live",
		Permissions: "0x1b",
	}).RawResponse
	require.NotNil(t, first)
	got := handler.HandleRaw(t.Context(), funcapi.RawMethodRequest{
		Method:      "rum-live",
		Args:        []string{"after:" + first["next"].(string)},
		Permissions: "0x1b",
	})
	assert.Equal(t, funcapi.ErrorResponse(400, `unsupported argument "after"`), got)
}
