// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/ceph/cephfunc"
)

func TestFuncDepsHealth(t *testing.T) {
	details := make([]map[string]any, 5)
	for i := range details {
		details[i] = map[string]any{"message": "detail"}
	}
	warnDetails := make([]map[string]any, cephfunc.MaxInventoryLimit)
	for i := range warnDetails {
		warnDetails[i] = map[string]any{"message": "warning detail"}
	}

	tests := map[string]struct {
		response        map[string]any
		limit           int
		wantTotal       int
		wantRows        int
		wantFirstID     string
		wantFirstCode   string
		wantFirstDetail string
		wantFirstCount  int
	}{
		"normalizes detail rows": {
			response: healthChecksResponse([]map[string]any{{
				"type":     "SLOW_OPS",
				"severity": "HEALTH_WARN",
				"muted":    false,
				"summary":  map[string]any{"message": "slow operations", "count": 2},
				"detail":   []map[string]any{{"message": "osd.1 slow"}, {"message": "osd.2 slow"}},
			}}),
			limit:           500,
			wantTotal:       2,
			wantRows:        2,
			wantFirstID:     "SLOW_OPS#00000000",
			wantFirstDetail: "osd.1 slow",
			wantFirstCount:  2,
		},
		"retains limit lookahead": {
			response: healthChecksResponse([]map[string]any{{
				"type":     "SLOW_OPS",
				"severity": "HEALTH_WARN",
				"summary":  map[string]any{"message": "slow", "count": 5},
				"detail":   details,
			}}),
			limit:     1,
			wantTotal: 5,
			wantRows:  2,
		},
		"returns severe bounded subset above inventory ceiling": {
			response: healthChecksResponse([]map[string]any{
				{
					"type":     "A_WARN",
					"severity": "HEALTH_WARN",
					"summary":  map[string]any{"message": "warning", "count": len(warnDetails)},
					"detail":   warnDetails,
				},
				{
					"type":     "Z_ERR",
					"severity": "HEALTH_ERR",
					"summary":  map[string]any{"message": "error", "count": 1},
					"detail":   []map[string]any{{"message": "error detail"}},
				},
			}),
			limit:         cephfunc.DefaultInventoryLimit,
			wantTotal:     cephfunc.MaxInventoryLimit + 1,
			wantRows:      cephfunc.DefaultInventoryLimit + 1,
			wantFirstCode: "Z_ERR",
		},
		"prioritizes errors before bound": {
			response: healthChecksResponse([]map[string]any{
				{"type": "A_WARN", "severity": "HEALTH_WARN", "summary": map[string]any{"message": "warn", "count": 1}},
				{"type": "B_WARN", "severity": "HEALTH_WARN", "summary": map[string]any{"message": "warn", "count": 1}},
				{"type": "Z_ERR", "severity": "HEALTH_ERR", "summary": map[string]any{"message": "error", "count": 1}},
			}),
			limit:         1,
			wantTotal:     3,
			wantRows:      2,
			wantFirstCode: "Z_ERR",
		},
		"accepts keyed compatibility form": {
			response: healthChecksResponse(map[string]any{
				"SLOW_OPS": map[string]any{
					"severity": "HEALTH_WARN",
					"summary":  map[string]any{"message": "slow", "count": 1},
				},
			}),
			limit:         500,
			wantTotal:     1,
			wantRows:      1,
			wantFirstCode: "SLOW_OPS",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != urlPathApiHealthMinimal {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				writeJSON(w, http.StatusOK, test.response)
			})
			defer srv.Close()
			c := newInitializedCollector(t, srv.URL, nil)
			defer c.Cleanup(context.Background())

			result, err := (funcDepsAdapter{
				collector: c,
			}).Health(context.Background(), test.limit)
			require.NoError(t, err)
			assert.Equal(t, test.wantTotal, result.Total)
			require.Len(t, result.Rows, test.wantRows)
			if test.wantFirstID != "" {
				assert.Equal(t, test.wantFirstID, result.Rows[0].ID)
			}
			if test.wantFirstCode != "" {
				assert.Equal(t, test.wantFirstCode, result.Rows[0].Code)
			}
			if test.wantFirstDetail != "" {
				assert.Equal(t, test.wantFirstDetail, result.Rows[0].Detail)
			}
			if test.wantFirstCount != 0 {
				assert.EqualValues(t, test.wantFirstCount, result.Rows[0].Count)
			}
		})
	}
}

func TestFunctionRevalidatesPinnedClusterIdentity(t *testing.T) {
	var fsid atomic.Value
	fsid.Store("cluster-a")
	var healthRequests atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathAPIClusterFSID:
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = fmt.Fprintf(w, "%q", fsid.Load().(string))
		case urlPathApiAuth:
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"token":"test-token"}`)
		case urlPathApiHealthMinimal:
			healthRequests.Add(1)
			_, _ = io.WriteString(w, `{"health":{"checks":[]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	require.NoError(t, c.Check(context.Background()))
	fsid.Store("cluster-b")

	_, err := (funcDepsAdapter{
		collector: c,
	}).Health(context.Background(), 500)
	require.ErrorContains(t, err, "cluster identity changed")
	assert.Zero(t, healthRequests.Load())
}

func TestFuncDepsHealthRejectsInvalidResponses(t *testing.T) {
	tests := map[string]struct {
		response map[string]any
		want     string
	}{
		"missing health": {
			response: map[string]any{},
			want:     "missing health checks",
		},
		"missing checks": {
			response: map[string]any{"health": map[string]any{}},
			want:     "missing health checks",
		},
		"duplicate type": {
			response: healthChecksResponse([]map[string]any{
				{"type": "SLOW_OPS", "severity": "HEALTH_WARN"},
				{"type": "SLOW_OPS", "severity": "HEALTH_WARN"},
			}),
		},
		"negative count": {
			response: healthChecksResponse([]map[string]any{
				{"type": "SLOW_OPS", "severity": "HEALTH_WARN", "summary": map[string]any{"count": -1}},
			}),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != urlPathApiHealthMinimal {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				writeJSON(w, http.StatusOK, test.response)
			})
			defer srv.Close()
			c := newInitializedCollector(t, srv.URL, nil)
			defer c.Cleanup(context.Background())

			_, err := (funcDepsAdapter{
				collector: c,
			}).Health(context.Background(), 500)
			if test.want != "" {
				require.ErrorContains(t, err, test.want)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func healthChecksResponse(checks any) map[string]any {
	return map[string]any{"health": map[string]any{"checks": checks}}
}

func TestFuncDepsOSDsRejectsInventoryAboveSelectedLimit(t *testing.T) {
	var requestedLimit string
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != urlPathApiOsd {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		requestedLimit = r.URL.Query().Get("limit")
		w.Header().Set("X-Total-Count", "5")
		rows := make([]map[string]any, 5)
		for i := range rows {
			rows[i] = map[string]any{"id": i, "uuid": fmt.Sprintf("uuid-%d", i)}
		}
		writeJSON(w, http.StatusOK, rows)
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())

	result, err := (funcDepsAdapter{
		collector: c,
	}).OSDs(context.Background(), 1)
	require.ErrorContains(t, err, "selected limit")
	assert.Equal(t, "1", requestedLimit)
	assert.Empty(t, result.Rows)
}

func TestOSDFunctionReportsIncompleteDashboardInventory(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != urlPathApiOsd {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("X-Total-Count", "2")
		writeJSON(w, http.StatusOK, []map[string]any{{"id": 1, "uuid": "uuid-1"}})
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	requireClusterIdentity(t, c)

	response := c.funcRouter.Handle(context.Background(), cephfunc.MethodOSDs, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, response.Status)
	assert.Equal(t, "Ceph OSD inventory is incomplete: received 1 of 2 rows", response.Message)
	assert.Nil(t, response.Data)
}

func TestFuncDepsPoolsAndCrushPlacement(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiPool:
			if r.URL.Query().Get("stats") != "false" ||
				!strings.Contains(r.URL.Query().Get("attrs"), "pg_autoscale_mode") {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, []map[string]any{
				{"pool_name": "pool-c"},
				{
					"pool_name": "pool-a", "type": "replicated", "size": 3, "min_size": 2,
					"pg_num": 32, "pg_placement_num": 32, "pg_autoscale_mode": "on",
					"crush_rule": "ssd-rule", "application_metadata": []string{"rgw"},
					"quota_max_bytes": 1024, "quota_max_objects": 100,
				},
				{"pool_name": "pool-b"},
			})
		case urlPathAPICrushRule:
			if r.Header.Get("Accept") != hdrAcceptVersionV2 {
				w.WriteHeader(http.StatusNotAcceptable)
				return
			}
			writeJSON(w, http.StatusOK, []map[string]any{{
				"rule_name": "ssd-rule",
				"steps": []map[string]any{
					{"op": "take", "item_name": "default~ssd"},
					{"op": "chooseleaf_firstn", "type": "host"},
				},
			}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())

	tests := map[string]struct {
		limit     int
		wantError string
		wantRows  int
	}{
		"rejects partial inventory":  {limit: 1, wantError: "selected limit"},
		"returns complete inventory": {limit: 3, wantRows: 3},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := (funcDepsAdapter{
				collector: c,
			}).Pools(context.Background(), test.limit)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				assert.Empty(t, result.Rows)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantRows, result.Total)
			require.Len(t, result.Rows, test.wantRows)
			rows := make(map[string]cephfunc.PoolRow, len(result.Rows))
			for _, row := range result.Rows {
				rows[row.Name] = row
			}
			require.Contains(t, rows, "pool-a")
			row := rows["pool-a"]
			assert.Equal(t, "pool-a", row.Name)
			assert.EqualValues(t, 3, *row.Size)
			assert.Equal(t, "default", row.CrushRoot)
			assert.Equal(t, "ssd", row.DeviceClass)
			assert.Equal(t, "host", row.FailureDomain)
			assert.Equal(t, "rgw", row.Applications)
			assert.Nil(t, rows["pool-b"].Size)
			assert.Nil(t, rows["pool-b"].PGNum)
		})
	}
}

func TestFuncDepsDaemonsRejectsPartialAndReturnsCompleteInventory(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != urlPathAPIDaemon {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, []map[string]any{
			{"daemon_name": "osd.9", "daemon_id": "9", "daemon_type": "osd", "is_active": true},
			{"daemon_name": "mon.a", "daemon_id": "a", "daemon_type": "mon", "is_active": true},
			{"daemon_name": "mgr.a", "daemon_id": "a", "daemon_type": "mgr", "is_active": true},
		})
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())

	tests := map[string]struct {
		limit     int
		wantError string
		wantRows  int
	}{
		"rejects partial inventory":  {limit: 1, wantError: "selected limit"},
		"returns complete inventory": {limit: 3, wantRows: 3},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := (funcDepsAdapter{
				collector: c,
			}).Daemons(context.Background(), test.limit)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				assert.Empty(t, result.Rows)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantRows, result.Total)
			require.Len(t, result.Rows, test.wantRows)
			ids := make([]string, 0, len(result.Rows))
			for _, row := range result.Rows {
				ids = append(ids, row.ID)
			}
			assert.ElementsMatch(t, []string{"mgr.a", "mon.a", "osd.9"}, ids)
		})
	}
}

func TestFuncDepsDaemonsKeepsMissingActiveStateUnknown(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == urlPathAPIDaemon {
			writeJSON(w, http.StatusOK, []map[string]any{{
				"daemon_name": "mon.a", "daemon_id": "a", "daemon_type": "mon",
			}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())

	result, err := (funcDepsAdapter{
		collector: c,
	}).Daemons(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)
	assert.Nil(t, result.Rows[0].Active)
}

func TestCollectorFunctionAvailability(t *testing.T) {
	c := New()
	tests := map[string]struct {
		method string
		want   bool
	}{
		"health":  {method: cephfunc.MethodHealth, want: true},
		"OSDs":    {method: cephfunc.MethodOSDs, want: true},
		"pools":   {method: cephfunc.MethodPools, want: true},
		"daemons": {method: cephfunc.MethodDaemons, want: true},
		"unknown": {method: "unknown"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, c.FunctionAvailable(test.method))
		})
	}
}
