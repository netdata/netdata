// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticfunc_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/query"
	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/syntheticfunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCataloguePaginationAndGlobalStatusWithoutHistoryOrJobs(t *testing.T) {
	ctx := context.Background()
	store, err := artifacts.Open(t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	prefix := publishReports(t, store, []captureOwner{
		{"journey:retired", "11111111-1111-4111-8111-111111111111"},
		{"lighthouse:renamed", "22222222-2222-4222-8222-222222222222"},
		{"journey:retired", "11111111-1111-4111-8111-111111111111"},
	})
	_, err = store.Enforce(ctx, 7, 1<<20)
	require.NoError(t, err)
	policyError := func() string { return "policy reload rejected" }
	h := syntheticfunc.New(query.New(syntheticregistry.New(), emptyHistory(t), store, policyError), nil)
	response := h.HandleRaw(
		ctx,
		funcapi.RawMethodRequest{
			Method:      "synthetics-artifacts",
			Permissions: "0x1b",
			Args:        []string{"limit:2"},
		},
	)
	rows := responseData(t, response)
	require.Len(t, rows, 2)
	assert.Equal(t, prefix+"3", rowValues(t, response, rows[0])["run_id"])
	assert.Equal(t, true, response.RawResponse["truncated"])
	next, ok := response.RawResponse["next"].(string)
	require.True(t, ok)
	validate(t, response)
	encoded, err := json.Marshal(response.RawResponse["storage"])
	require.NoError(t, err)
	var status map[string]any
	require.NoError(t, json.Unmarshal(encoded, &status))
	assert.Equal(t, "policy reload rejected", status["policy_error"])
	assert.Equal(t, float64(7), status["policy"].(map[string]any)["days"])
	inventory := status["inventory"].(map[string]any)
	assert.Equal(t, float64(3), inventory["known_sets"])
	assert.Equal(t, float64(2), inventory["known_jobs"])
	assert.NotNil(t, inventory["retained"].(map[string]any)["bytes"])
	assert.Equal(t, float64(0), inventory["workspace"].(map[string]any)["bytes"])
	page2 := call(h, "synthetics-artifacts", "limit:2", "cursor:"+next)
	rows = responseData(t, page2)
	require.Len(t, rows, 1)
	assert.Equal(t, prefix+"1", rowValues(t, page2, rows[0])["run_id"])
	assert.Equal(t, false, page2.RawResponse["truncated"])
	assert.NotContains(t, page2.RawResponse, "next")
	validate(t, page2)
	filtered := call(h, "synthetics-artifacts", "job_id:lighthouse:renamed", "limit:1")
	require.Len(t, responseData(t, filtered), 1)
	sitePage := call(h, "synthetics-artifacts", "vnode_guid:11111111-1111-4111-8111-111111111111", "limit:1")
	siteRows := responseData(t, sitePage)
	require.Len(t, siteRows, 1)
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", rowValues(t, sitePage, siteRows[0])["vnode_guid"])
	assert.Equal(t, prefix+"3", rowValues(t, sitePage, siteRows[0])["run_id"])
	validate(t, sitePage)
	siteNext := call(
		h,
		"synthetics-artifacts",
		"vnode_guid:11111111-1111-4111-8111-111111111111",
		"limit:1",
		"cursor:"+sitePage.RawResponse["next"].(string),
	)
	siteRows = responseData(t, siteNext)
	require.Len(t, siteRows, 1)
	assert.Equal(t, prefix+"1", rowValues(t, siteNext, siteRows[0])["run_id"])
	assert.Empty(
		t,
		responseData(
			t,
			call(
				h,
				"synthetics-artifacts",
				"vnode_guid:11111111-1111-4111-8111-111111111111",
				"job_id:lighthouse:renamed",
			),
		),
	)
	detail := call(h, "synthetics-artifact", "job_id:journey:retired", "run_id:"+prefix+"1")
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", detail.RawResponse["vnode_guid"])
	filteredStatus, err := json.Marshal(filtered.RawResponse["storage"])
	require.NoError(t, err)
	var filteredFields map[string]any
	require.NoError(t, json.Unmarshal(filteredStatus, &filteredFields))
	filteredInventory := filteredFields["inventory"].(map[string]any)
	assert.Equal(t, inventory["retained"], filteredInventory["retained"])
	assert.Equal(t, inventory["known_sets"], filteredInventory["known_sets"])
	assert.Equal(t, inventory["known_jobs"], filteredInventory["known_jobs"])
	for _, args := range [][]string{{"limit:0"}, {"limit:2001"}, {"cursor:123"}, {"cursor:0.00000000000000000000000000000001"}, {"cursor:1.unsafe/path"}, {"cursor:+1.00000000000000000000000000000001"}, {"job_id:invalid"}} {
		assert.Equal(t, 400, call(h, "synthetics-artifacts", args...).Status, args)
	}
	denied := h.HandleRaw(ctx, funcapi.RawMethodRequest{
		Method:      "synthetics-artifacts",
		Permissions: "0x19",
	})
	assert.Equal(t, 403, denied.Status)
}

// captureOwner is the job and vnode GUID a published capture set belongs to.
type captureOwner struct{ jobID, vnodeGUID string }

// publishReports publishes one report set per owner, in order, and returns the shared run-ID prefix. Set i has run
// ID prefix+hex(i+1): ascending suffixes keep run-ID order equal to publication order, which page assertions rely on.
func publishReports(t *testing.T, store *artifacts.Store, owners []captureOwner) string {
	t.Helper()
	prefix := fixtureRunID()[:31]
	for i, owner := range owners {
		runID := fmt.Sprintf("%s%x", prefix, i+1)
		work, err := store.Begin(runID, owner.jobID, owner.vnodeGUID)
		require.NoError(t, err)
		require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(work, "output/report.html"), []byte("<html>report</html>"), 0600))
		_, err = store.Finalize(
			t.Context(),
			runID,
			[]synthetic.Capture{{ID: "report", Kind: "report", MIME: "text/html", Path: "output/report.html"}},
		)
		require.NoError(t, err)
	}
	return prefix
}

func TestCatalogueCursorPagesThroughArgumentsAndTheCloudTableBody(t *testing.T) {
	store, err := artifacts.Open(t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	prefix := publishReports(t, store, []captureOwner{{"journey:a", ""}, {"journey:a", ""}, {"journey:a", ""}})
	h := syntheticfunc.New(query.New(syntheticregistry.New(), emptyHistory(t), store, noPolicyError), nil)
	declarations := syntheticfunc.Declarations()
	decl := declarations[slices.IndexFunc(declarations, func(d funcapi.FunctionConfig) bool {
		return d.ID == "synthetics-artifacts"
	})]
	for name, request := range map[string]func(cursor string) funcapi.RawMethodRequest{
		"arguments": func(cursor string) funcapi.RawMethodRequest {
			args := []string{"limit:1"}
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
			fields := map[string]any{"limit": 1}
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
			var runs []any
			cursor := ""
			for range 3 {
				got := h.HandleRaw(t.Context(), request(cursor))
				rows := responseData(t, got)
				require.Len(t, rows, 1)
				runs = append(runs, rowValues(t, got, rows[0])["run_id"])
				next, _ := got.RawResponse["next"].(string)
				assert.Equal(t, next != "", got.RawResponse["truncated"])
				cursor = next
			}
			assert.Equal(t, []any{prefix + "3", prefix + "2", prefix + "1"}, runs)
			assert.Empty(t, cursor, "the last page returns no next cursor")
		})
	}
	// The former parameter name is not a cursor alias.
	first := call(h, decl.ID, "limit:1")
	assert.Equal(
		t,
		funcapi.ErrorResponse(400, `unsupported argument "before"`),
		call(h, decl.ID, "limit:1", "before:"+first.RawResponse["next"].(string)),
	)
}
