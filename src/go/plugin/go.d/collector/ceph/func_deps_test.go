// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
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
		writeJSON(t, w, http.StatusOK, map[string]any{
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
		writeJSON(t, w, http.StatusOK, map[string]any{
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
		writeJSON(t, w, http.StatusOK, map[string]any{
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
		writeJSON(t, w, http.StatusOK, map[string]any{
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
				writeJSON(t, w, http.StatusOK, response)
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
					writeJSON(t, w, http.StatusOK, map[string]any{"health": map[string]any{"checks": checks}})
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

func TestFuncDepsOSDsDefensivelyBoundsOversizedPage(t *testing.T) {
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
		writeJSON(t, w, http.StatusOK, rows)
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())

	result, err := (funcDepsAdapter{collector: c}).OSDs(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "2", requestedLimit)
	assert.Equal(t, 5, result.Total)
	assert.Len(t, result.Rows, 2)
}

func TestFuncDepsPoolsAndCrushPlacement(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiPool:
			assert.Equal(t, "false", r.URL.Query().Get("stats"))
			assert.Contains(t, r.URL.Query().Get("attrs"), "pg_autoscale_mode")
			writeJSON(t, w, http.StatusOK, []map[string]any{
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
			assert.Equal(t, hdrAcceptVersionV2, r.Header.Get("Accept"))
			writeJSON(t, w, http.StatusOK, []map[string]any{{
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
	require.NoError(t, err)
	assert.Equal(t, 3, result.Total)
	require.Len(t, result.Rows, 2)
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

func TestFuncDepsDaemonsSelectsDeterministicallyBeforeBound(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != urlPathAPIDaemon {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(t, w, http.StatusOK, []map[string]any{
			{"daemon_name": "osd.9", "daemon_id": "9", "daemon_type": "osd", "is_active": true},
			{"daemon_name": "mon.a", "daemon_id": "a", "daemon_type": "mon", "is_active": true},
			{"daemon_name": "mgr.a", "daemon_id": "a", "daemon_type": "mgr", "is_active": true},
		})
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())

	result, err := (funcDepsAdapter{collector: c}).Daemons(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Total)
	require.Len(t, result.Rows, 2)
	assert.Equal(t, "mgr.a", result.Rows[0].ID)
	assert.Equal(t, "mon.a", result.Rows[1].ID)
}

func TestFuncDepsDaemonsKeepsMissingActiveStateUnknown(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == urlPathAPIDaemon {
			writeJSON(t, w, http.StatusOK, []map[string]any{{
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

func TestFuncDepsRGWQuotasUsesOnlyExplicitPointLookups(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case urlPathAPIRGWUser + "/tenant$user-a":
			assert.Equal(t, "true", r.URL.Query().Get("stats"))
			writeJSON(t, w, http.StatusOK, map[string]any{
				"uid": "tenant$user-a", "tenant": "tenant-a", "keys": []map[string]any{{"secret_key": "test-secret"}},
				"stats":      map[string]any{"size_actual": 50, "num_objects": 5},
				"user_quota": map[string]any{"enabled": true, "max_size": 100, "max_objects": 10},
			})
		case urlPathAPIRGWBucket + "/bucket-a":
			writeJSON(t, w, http.StatusOK, map[string]any{
				"bucket": "bucket-a", "owner": "tenant$user-a",
				"usage":        map[string]any{"rgw.main": map[string]any{"size_actual": 25, "num_objects": 2}},
				"bucket_quota": map[string]any{"enabled": true, "max_size": 100, "max_objects": 20},
			})
		case urlPathAPIRGWAccounts + "/account-a":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, func(c *Collector) {
		c.Functions.RGWQuotas.Users = []string{"tenant$user-a"}
		c.Functions.RGWQuotas.Buckets = []string{"bucket-a"}
		c.Functions.RGWQuotas.Accounts = []string{"account-a"}
	})
	defer c.Cleanup(context.Background())

	result, err := (funcDepsAdapter{collector: c}).RGWQuotas(context.Background(), 100)
	require.NoError(t, err)
	require.Len(t, result.Rows, 3)
	rows := make(map[string]cephfunc.RGWQuotaRow)
	for _, row := range result.Rows {
		rows[row.Kind] = row
		assert.Equal(t, row.Kind+":"+row.ID, row.Key)
	}
	assert.Equal(t, "unsupported_or_not_found", rows["account"].Status)
	assert.EqualValues(t, 50, *rows["user"].UsedBytes)
	assert.EqualValues(t, 50, *rows["user"].Utilization)
	assert.EqualValues(t, 25, *rows["bucket"].UsedBytes)
	assert.EqualValues(t, 25, *rows["bucket"].Utilization)

	mu.Lock()
	sort.Strings(paths)
	gotPaths := append([]string(nil), paths...)
	mu.Unlock()
	assert.Equal(t, []string{
		urlPathAPIRGWAccounts + "/account-a",
		urlPathAPIRGWBucket + "/bucket-a",
		urlPathAPIRGWUser + "/tenant$user-a",
	}, gotPaths)
	assert.NotContains(t, gotPaths, urlPathAPIRGWUser)
}

func TestFuncDepsRGWQuotaKeysRemainUniqueAcrossKinds(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathAPIRGWUser + "/shared":
			writeJSON(t, w, http.StatusOK, map[string]any{})
		case urlPathAPIRGWBucket + "/shared":
			writeJSON(t, w, http.StatusOK, map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, func(c *Collector) {
		c.Functions.RGWQuotas.Users = []string{"shared"}
		c.Functions.RGWQuotas.Buckets = []string{"shared"}
	})
	defer c.Cleanup(context.Background())

	result, err := (funcDepsAdapter{collector: c}).RGWQuotas(context.Background(), 100)
	require.NoError(t, err)
	require.Len(t, result.Rows, 2)
	assert.Equal(t, "bucket:shared", result.Rows[0].Key)
	assert.Equal(t, "user:shared", result.Rows[1].Key)
	assert.Equal(t, "shared", result.Rows[0].ID)
	assert.Equal(t, "shared", result.Rows[1].ID)
}

func TestFuncDepsRGWQuotasDoesNotFabricateMalformedUsageAsZero(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == urlPathAPIRGWUser+"/user-a" {
			writeJSON(t, w, http.StatusOK, map[string]any{
				"stats":      map[string]any{"size_actual": 1.5, "num_objects": 1},
				"user_quota": map[string]any{"enabled": true, "max_size": 100, "max_objects": 10},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, func(c *Collector) {
		c.Functions.RGWQuotas.Users = []string{"user-a"}
	})
	defer c.Cleanup(context.Background())

	result, err := (funcDepsAdapter{collector: c}).RGWQuotas(context.Background(), 100)
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)
	assert.Equal(t, "error", result.Rows[0].Status)
	assert.Nil(t, result.Rows[0].UsedBytes)
}

func TestFuncDepsRGWMultisiteBoundsAndSortsSyncProbes(t *testing.T) {
	var probed []string
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathAPIRGWRealm:
			writeJSON(t, w, http.StatusOK, map[string]any{"realms": []string{}})
		case urlPathAPIRGWZonegroup:
			writeJSON(t, w, http.StatusOK, map[string]any{"zonegroups": []string{}})
		case urlPathAPIRGWZone:
			writeJSON(t, w, http.StatusOK, map[string]any{"zones": []string{}})
		case urlPathAPIRGWDaemon:
			daemons := make([]map[string]any, 12)
			for i := range daemons {
				daemons[i] = map[string]any{"id": fmt.Sprintf("rgw-%02d", 11-i)}
			}
			writeJSON(t, w, http.StatusOK, daemons)
		case urlPathAPIRGWSyncStatus:
			probed = append(probed, r.URL.Query().Get("daemon_name"))
			writeJSON(t, w, http.StatusOK, map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())

	result, err := (funcDepsAdapter{collector: c}).RGWMultisite(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, maxRGWSyncProbes, result.Total)
	assert.Len(t, result.Rows, maxRGWSyncProbes)
	assert.Equal(t, []string{
		"rgw-00", "rgw-01", "rgw-02", "rgw-03", "rgw-04",
		"rgw-05", "rgw-06", "rgw-07", "rgw-08", "rgw-09",
	}, probed)
}

func TestFuncDepsRGWMultisiteUsesTopologyOrderAndMasterRelationships(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathAPIRGWRealm:
			writeJSON(t, w, http.StatusOK, map[string]any{"realms": []string{"realm-a"}, "default_info": "realm-id"})
		case urlPathAPIRGWRealm + "/realm-a":
			writeJSON(t, w, http.StatusOK, map[string]any{"id": "realm-id", "name": "realm-a"})
		case urlPathAPIRGWZonegroup:
			writeJSON(t, w, http.StatusOK, map[string]any{"zonegroups": []string{"zonegroup-a"}, "default_info": "zonegroup-id"})
		case urlPathAPIRGWZonegroup + "/zonegroup-a":
			writeJSON(t, w, http.StatusOK, map[string]any{
				"id": "zonegroup-id", "name": "zonegroup-a", "realm_id": "realm-id",
				"is_master": true, "master_zone": "zone-id",
			})
		case urlPathAPIRGWZone:
			writeJSON(t, w, http.StatusOK, map[string]any{"zones": []string{"zone-a"}, "default_info": "zone-id"})
		case urlPathAPIRGWZone + "/zone-a":
			writeJSON(t, w, http.StatusOK, map[string]any{
				"id": "zone-id", "name": "zone-a", "realm_id": "realm-id", "zonegroup_id": "zonegroup-id",
			})
		case urlPathAPIRGWDaemon:
			writeJSON(t, w, http.StatusOK, []map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())

	result, err := (funcDepsAdapter{collector: c}).RGWMultisite(context.Background(), 100)
	require.NoError(t, err)
	require.Len(t, result.Rows, 3)
	assert.Equal(t, []string{"realm", "zonegroup", "zone"}, []string{
		result.Rows[0].Kind, result.Rows[1].Kind, result.Rows[2].Kind,
	})
	assert.Equal(t, boolPtr(true), result.Rows[0].Default)
	assert.Nil(t, result.Rows[0].Master)
	assert.Equal(t, boolPtr(true), result.Rows[1].Default)
	assert.Equal(t, boolPtr(true), result.Rows[1].Master)
	assert.Equal(t, boolPtr(true), result.Rows[2].Default)
	assert.Equal(t, boolPtr(true), result.Rows[2].Master)
}

func TestFuncDepsRGWMultisiteDistinguishesEmptyFromAbsentRelationships(t *testing.T) {
	for _, test := range []struct {
		name  string
		known bool
	}{
		{name: "explicit empty means none", known: true},
		{name: "absent remains unknown", known: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
				list := func(key, name string) map[string]any {
					value := map[string]any{key: []string{name}}
					if test.known {
						value["default_info"] = ""
					}
					return value
				}
				switch r.URL.Path {
				case urlPathAPIRGWRealm:
					writeJSON(t, w, http.StatusOK, list("realms", "realm-a"))
				case urlPathAPIRGWRealm + "/realm-a":
					writeJSON(t, w, http.StatusOK, map[string]any{"id": "realm-id", "name": "realm-a"})
				case urlPathAPIRGWZonegroup:
					writeJSON(t, w, http.StatusOK, list("zonegroups", "zonegroup-a"))
				case urlPathAPIRGWZonegroup + "/zonegroup-a":
					detail := map[string]any{
						"id": "zonegroup-id", "name": "zonegroup-a", "realm_id": "realm-id",
					}
					if test.known {
						detail["is_master"] = false
						detail["master_zone"] = ""
					}
					writeJSON(t, w, http.StatusOK, detail)
				case urlPathAPIRGWZone:
					writeJSON(t, w, http.StatusOK, list("zones", "zone-a"))
				case urlPathAPIRGWZone + "/zone-a":
					writeJSON(t, w, http.StatusOK, map[string]any{
						"id": "zone-id", "name": "zone-a", "realm_id": "realm-id", "zonegroup_id": "zonegroup-id",
					})
				case urlPathAPIRGWDaemon:
					writeJSON(t, w, http.StatusOK, []map[string]any{})
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})
			defer srv.Close()
			c := newInitializedCollector(t, srv.URL, nil)
			defer c.Cleanup(context.Background())

			result, err := (funcDepsAdapter{collector: c}).RGWMultisite(context.Background(), 100)
			require.NoError(t, err)
			require.Len(t, result.Rows, 3)
			if test.known {
				assert.Equal(t, boolPtr(false), result.Rows[0].Default)
				assert.Equal(t, boolPtr(false), result.Rows[1].Default)
				assert.Equal(t, boolPtr(false), result.Rows[1].Master)
				assert.Equal(t, boolPtr(false), result.Rows[2].Default)
				assert.Equal(t, boolPtr(false), result.Rows[2].Master)
			} else {
				assert.Nil(t, result.Rows[0].Default)
				assert.Nil(t, result.Rows[1].Default)
				assert.Nil(t, result.Rows[1].Master)
				assert.Nil(t, result.Rows[2].Default)
				assert.Nil(t, result.Rows[2].Master)
			}
		})
	}
}

func TestFuncDepsRGWMultisiteReportsDaemonInventoryFailure(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathAPIRGWRealm:
			writeJSON(t, w, http.StatusOK, map[string]any{"realms": []string{}})
		case urlPathAPIRGWZonegroup:
			writeJSON(t, w, http.StatusOK, map[string]any{"zonegroups": []string{}})
		case urlPathAPIRGWZone:
			writeJSON(t, w, http.StatusOK, map[string]any{"zones": []string{}})
		case urlPathAPIRGWDaemon:
			w.WriteHeader(http.StatusForbidden)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()
	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())

	result, err := (funcDepsAdapter{collector: c}).RGWMultisite(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	require.Len(t, result.Rows, 1)
	assert.Equal(t, "permission_denied", result.Rows[0].SyncStatus)
}

func TestCollectorFunctionAvailability(t *testing.T) {
	c := New()
	assert.True(t, c.FunctionAvailable(cephfunc.MethodHealth))
	assert.True(t, c.FunctionAvailable(cephfunc.MethodOSDs))
	assert.True(t, c.FunctionAvailable(cephfunc.MethodPools))
	assert.True(t, c.FunctionAvailable(cephfunc.MethodDaemons))
	assert.False(t, c.FunctionAvailable(cephfunc.MethodRGWMultisite))
	assert.False(t, c.FunctionAvailable(cephfunc.MethodRGWQuotas))
	assert.False(t, c.FunctionAvailable("unknown"))
}
