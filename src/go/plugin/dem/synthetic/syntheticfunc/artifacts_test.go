// SPDX-License-Identifier: GPL-3.0-or-later
package syntheticfunc_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	// Ascending suffixes keep run-ID order equal to publication order, which the page assertions rely on.
	prefix := fixtureRunID()[:31]
	for i, jobID := range []string{"journey:retired", "lighthouse:renamed", "journey:retired"} {
		runID := fmt.Sprintf("%s%x", prefix, i+1)
		work, err := store.Begin(runID, jobID, []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "11111111-1111-4111-8111-111111111111"}[i])
		require.NoError(t, err)
		require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(work, "output/report.html"), []byte("<html>report</html>"), 0600))
		_, err = store.Finalize(
			ctx,
			runID,
			[]synthetic.Capture{{ID: "report", Kind: "report", MIME: "text/html", Path: "output/report.html"}},
		)
		require.NoError(t, err)
	}
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
	page2 := call(h, "synthetics-artifacts", "limit:2", "before:"+next)
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
	siteNext := call(h, "synthetics-artifacts", "vnode_guid:11111111-1111-4111-8111-111111111111", "limit:1", "before:"+sitePage.RawResponse["next"].(string))
	siteRows = responseData(t, siteNext)
	require.Len(t, siteRows, 1)
	assert.Equal(t, prefix+"1", rowValues(t, siteNext, siteRows[0])["run_id"])
	assert.Empty(t, responseData(t, call(h, "synthetics-artifacts", "vnode_guid:11111111-1111-4111-8111-111111111111", "job_id:lighthouse:renamed")))
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
	for _, args := range [][]string{{"limit:0"}, {"limit:2001"}, {"before:123"}, {"before:0.00000000000000000000000000000001"}, {"before:1.unsafe/path"}, {"before:+1.00000000000000000000000000000001"}, {"job_id:invalid"}} {
		assert.Equal(t, 400, call(h, "synthetics-artifacts", args...).Status, args)
	}
	denied := h.HandleRaw(ctx, funcapi.RawMethodRequest{
		Method:      "synthetics-artifacts",
		Permissions: "0x19",
	})
	assert.Equal(t, 403, denied.Status)
}
