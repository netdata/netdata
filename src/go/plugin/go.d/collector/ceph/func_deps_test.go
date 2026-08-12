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
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != urlPathApiHealthMinimal {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"health": map[string]any{"checks": []map[string]any{
				{
					"type":     "SLOW_OPS",
					"severity": "HEALTH_WARN", "muted": false,
					"summary": map[string]any{"message": "slow operations", "count": 2},
					"detail":  []map[string]any{{"message": "osd.1 slow"}, {"message": "osd.2 slow"}},
				},
			}},
		})
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())

	result, err := (funcDepsAdapter{collector: c}).Health(context.Background(), 500)
	require.NoError(t, err)
	require.Len(t, result.Rows, 2)
	assert.Equal(t, 2, result.Total)
	assert.Equal(t, "SLOW_OPS#00000000", result.Rows[0].ID)
	assert.Equal(t, "osd.1 slow", result.Rows[0].Detail)
	assert.EqualValues(t, 2, result.Rows[0].Count)
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

	_, err := (funcDepsAdapter{collector: c}).Health(context.Background(), 500)
	require.ErrorContains(t, err, "cluster identity changed")
	assert.Zero(t, healthRequests.Load())
}

func TestFuncDepsHealthBoundsNormalizedRows(t *testing.T) {
	details := make([]map[string]any, 5)
	for i := range details {
		details[i] = map[string]any{"message": "detail"}
	}
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != urlPathApiHealthMinimal {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"health": map[string]any{"checks": []map[string]any{
				{
					"type":     "SLOW_OPS",
					"severity": "HEALTH_WARN", "summary": map[string]any{"message": "slow", "count": 5},
					"detail": details,
				},
			}},
		})
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())

	result, err := (funcDepsAdapter{collector: c}).Health(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, 5, result.Total)
	assert.Len(t, result.Rows, 2)
}

func TestFuncDepsHealthPrioritizesErrorsBeforeBound(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != urlPathApiHealthMinimal {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"health": map[string]any{"checks": []map[string]any{
				{"type": "A_WARN", "severity": "HEALTH_WARN", "summary": map[string]any{"message": "warn", "count": 1}},
				{"type": "B_WARN", "severity": "HEALTH_WARN", "summary": map[string]any{"message": "warn", "count": 1}},
				{"type": "Z_ERR", "severity": "HEALTH_ERR", "summary": map[string]any{"message": "error", "count": 1}},
			}},
		})
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())

	result, err := (funcDepsAdapter{collector: c}).Health(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Total)
	require.Len(t, result.Rows, 2)
	assert.Equal(t, "Z_ERR", result.Rows[0].Code)
}

func TestFuncDepsHealthAcceptsKeyedCompatibilityForm(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != urlPathApiHealthMinimal {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"health": map[string]any{"checks": map[string]any{
				"SLOW_OPS": map[string]any{
					"severity": "HEALTH_WARN",
					"summary":  map[string]any{"message": "slow", "count": 1},
				},
			}},
		})
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())

	result, err := (funcDepsAdapter{collector: c}).Health(context.Background(), 500)
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)
	assert.Equal(t, "SLOW_OPS", result.Rows[0].Code)
}

func TestFuncDepsHealthRejectsMissingChecks(t *testing.T) {
	for _, response := range []map[string]any{
		{},
		{"health": map[string]any{}},
	} {
		t.Run(fmt.Sprint(response), func(t *testing.T) {
			srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != urlPathApiHealthMinimal {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				writeJSON(w, http.StatusOK, response)
			})
			defer srv.Close()
			c := newInitializedCollector(t, srv.URL, nil)
			defer c.Cleanup(context.Background())

			_, err := (funcDepsAdapter{collector: c}).Health(context.Background(), 500)
			require.ErrorContains(t, err, "missing health checks")
		})
	}
}

func TestFuncDepsHealthRejectsAmbiguousOrInvalidRows(t *testing.T) {
	for name, checks := range map[string][]map[string]any{
		"duplicate type": {
			{"type": "SLOW_OPS", "severity": "HEALTH_WARN"},
			{"type": "SLOW_OPS", "severity": "HEALTH_WARN"},
		},
		"negative count": {
			{"type": "SLOW_OPS", "severity": "HEALTH_WARN", "summary": map[string]any{"count": -1}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == urlPathApiHealthMinimal {
					writeJSON(w, http.StatusOK, map[string]any{"health": map[string]any{"checks": checks}})
					return
				}
				w.WriteHeader(http.StatusNotFound)
			})
			defer srv.Close()
			c := newInitializedCollector(t, srv.URL, nil)
			defer c.Cleanup(context.Background())

			_, err := (funcDepsAdapter{collector: c}).Health(context.Background(), 500)
			require.Error(t, err)
		})
	}
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

	result, err := (funcDepsAdapter{collector: c}).OSDs(context.Background(), 1)
	require.ErrorContains(t, err, "selected limit")
	assert.Equal(t, "1", requestedLimit)
	assert.Empty(t, result.Rows)
}

func TestFuncDepsPoolsAndCrushPlacement(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiPool:
			if r.URL.Query().Get("stats") != "false" || !strings.Contains(r.URL.Query().Get("attrs"), "pg_autoscale_mode") {
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
				"steps":     []map[string]any{{"op": "take", "item_name": "default~ssd"}, {"op": "chooseleaf_firstn", "type": "host"}},
			}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())

	result, err := (funcDepsAdapter{collector: c}).Pools(context.Background(), 1)
	require.ErrorContains(t, err, "selected limit")
	assert.Empty(t, result.Rows)

	result, err = (funcDepsAdapter{collector: c}).Pools(context.Background(), 3)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Total)
	require.Len(t, result.Rows, 3)
	row := result.Rows[0]
	assert.Equal(t, "pool-a", row.Name)
	assert.EqualValues(t, 3, *row.Size)
	assert.Equal(t, "default", row.CrushRoot)
	assert.Equal(t, "ssd", row.DeviceClass)
	assert.Equal(t, "host", row.FailureDomain)
	assert.Equal(t, "rgw", row.Applications)
	assert.Nil(t, result.Rows[1].Size)
	assert.Nil(t, result.Rows[1].PGNum)
}

func TestFuncDepsDaemonsRejectsPartialAndSortsCompleteInventory(t *testing.T) {
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

	result, err := (funcDepsAdapter{collector: c}).Daemons(context.Background(), 1)
	require.ErrorContains(t, err, "selected limit")
	assert.Empty(t, result.Rows)

	result, err = (funcDepsAdapter{collector: c}).Daemons(context.Background(), 3)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Total)
	require.Len(t, result.Rows, 3)
	assert.Equal(t, "mgr.a", result.Rows[0].ID)
	assert.Equal(t, "mon.a", result.Rows[1].ID)
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

	result, err := (funcDepsAdapter{collector: c}).Daemons(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)
	assert.Nil(t, result.Rows[0].Active)
}

func TestCollectorFunctionAvailability(t *testing.T) {
	c := New()
	assert.True(t, c.FunctionAvailable(cephfunc.MethodHealth))
	assert.True(t, c.FunctionAvailable(cephfunc.MethodOSDs))
	assert.True(t, c.FunctionAvailable(cephfunc.MethodPools))
	assert.True(t, c.FunctionAvailable(cephfunc.MethodDaemons))
	assert.False(t, c.FunctionAvailable("unknown"))
}
