// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"bufio"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

// Catalog caching (load-once, retry-after-failure, disabled-under-test) is now
// provided and tested by pkg/profilecatalog (Cached); it is not re-tested here.

// TestDefaultCatalog_AllStockProfilesHydrate hydrates and validates every stock
// profile's lazy fields. Profiles are hydrated lazily at runtime (only when a job
// selects one), so this test is what keeps a broken stock profile from
// slipping through CI — it would otherwise surface only when a job happens to
// select that profile. Numeric dimension options must also fit supported
// 32-bit Agents even when CI itself runs on a 64-bit host.
func TestDefaultCatalog_AllStockProfilesHydrate(t *testing.T) {
	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)

	profiles := catalog.OrderedProfiles()
	require.NotEmpty(t, profiles, "expected at least one stock profile")

	for _, p := range profiles {
		template, err := p.Template()
		require.NoErrorf(t, err, "stock profile %q template must be valid", p.Name)
		_, err = p.Relabeling()
		require.NoErrorf(t, err, "stock profile %q relabeling must be valid", p.Name)
		_, err = p.FallbackType()
		require.NoErrorf(t, err, "stock profile %q fallback_type must be valid", p.Name)

		const (
			minInt32 = int64(-1 << 31)
			maxInt32 = int64(1<<31 - 1)
		)
		var walk func(group charttpl.Group)
		walk = func(group charttpl.Group) {
			for _, chart := range group.Charts {
				for _, dimension := range chart.Dimensions {
					if dimension.Options == nil {
						continue
					}
					require.GreaterOrEqualf(t, int64(dimension.Options.Multiplier), minInt32,
						"stock profile %q selector %q multiplier must fit a signed 32-bit Agent", p.Name, dimension.Selector)
					require.LessOrEqualf(t, int64(dimension.Options.Multiplier), maxInt32,
						"stock profile %q selector %q multiplier must fit a signed 32-bit Agent", p.Name, dimension.Selector)
					require.GreaterOrEqualf(t, int64(dimension.Options.Divisor), minInt32,
						"stock profile %q selector %q divisor must fit a signed 32-bit Agent", p.Name, dimension.Selector)
					require.LessOrEqualf(t, int64(dimension.Options.Divisor), maxInt32,
						"stock profile %q selector %q divisor must fit a signed 32-bit Agent", p.Name, dimension.Selector)
				}
			}
			for _, child := range group.Groups {
				walk(child)
			}
		}
		walk(template)
	}
}

// TestDefaultCatalog_VLLMRayDeniesEveryCompatibilityGauge derives Ray 2.48's
// unsuffixed compatibility gauges from the source-union fixture. The profile
// also suppresses vLLM's deprecated pre-canonical KV-offload families so every
// operation is represented exactly once.
func TestDefaultCatalog_VLLMRayDeniesEveryCompatibilityGauge(t *testing.T) {
	file, err := os.Open("../testdata/vllm_ray_all_metrics.prom")
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()

	types := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 4 && fields[0] == "#" && fields[1] == "TYPE" {
			types[fields[2]] = fields[3]
		}
	}
	require.NoError(t, scanner.Err())

	var aliases []string
	for name, typ := range types {
		if typ == "gauge" && types[name+"_total"] == "counter" {
			aliases = append(aliases, name)
		}
	}
	slices.Sort(aliases)
	require.Len(t, aliases, 33)

	denials := append(aliases,
		"ray_vllm_kv_offload_size*",
		"ray_vllm_kv_offload_total_bytes_total",
		"ray_vllm_kv_offload_total_time_total",
	)
	slices.Sort(denials)

	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)
	profile, ok := catalog.Get("vllm_ray")
	require.True(t, ok)
	selector := profile.AutogenSelector()
	require.NotNil(t, selector)
	require.Equal(t, denials, selector.Deny)
}

func TestDefaultCatalog_HolisticProfilesHaveClosedFallbackAllowlists(t *testing.T) {
	want := map[string][]string{
		"ceph":     {"ceph_health_status", "ceph_daemon_socket_up", "ceph_nvmeof_gateway_info"},
		"litellm":  {"litellm_proxy_total_requests_metric_total"},
		"vllm":     {"vllm:num_requests_running"},
		"vllm_ray": {"ray_vllm_num_requests_running"},
	}

	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)
	for name, expected := range want {
		profile, ok := catalog.Get(name)
		require.Truef(t, ok, "stock profile %q must exist", name)
		selector := profile.AutogenSelector()
		require.NotNilf(t, selector, "stock profile %q must define fallback policy", name)
		require.Equalf(t, expected, selector.Allow,
			"stock profile %q must suppress unknown unmatched families", name)
	}
}

// TestDefaultCatalog_CephAlgorithmsFollowSourceLifecycle locks the cases where
// Ceph's Prometheus wire type is not the value lifecycle. Increment/decrement
// populations are current state; cumulative gauges and snapshots are raw totals
// that Netdata must rate and reset-detect.
func TestDefaultCatalog_CephAlgorithmsFollowSourceLifecycle(t *testing.T) {
	type expectation struct {
		algorithm string
		units     string
		divisor   int
		float     bool
	}
	want := map[string]expectation{
		"ceph_bluefs_read_zeros_candidate":                      {"incremental", "reads/s", 0, false},
		"ceph_bluestore_omap_iterator_count":                    {"absolute", "iterators", 0, false},
		"ceph_mds_log_evlrg":                                    {"incremental", "events/s", 0, false},
		"ceph_mds_inodes_expired":                               {"incremental", "inodes/s", 0, false},
		"ceph_mds_sessions_mdthresh_evicted":                    {"incremental", "sessions/s", 0, false},
		"ceph_mds_per_client_total_read_ops":                    {"incremental", "operations/s", 0, false},
		"ceph_AsyncMessenger_RDMADispatcher_active_queue_pair":  {"absolute", "queue pairs", 0, false},
		"ceph_AsyncMessenger_RDMADispatcher_inflight_tx_chunks": {"absolute", "chunks", 0, false},
		"ceph_AsyncMessenger_RDMADispatcher_rx_bufs_in_use":     {"absolute", "buffers", 0, false},
		"ceph_osd_cached_crc":                                   {"incremental", "lookups/s", 0, false},
		"ceph_oft_omap_total_updates":                           {"incremental", "operations/s", 0, false},
		"ceph_dmclock_scheduler_throttle":                       {"incremental", "requests/s", 0, false},
		"ceph_dmclock_scheduler_outstanding":                    {"absolute", "requests", 0, false},
		"ceph_client_mdsqsum":                                   {"incremental", "microseconds²/s", 1000000, true},
	}

	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)
	profile, ok := catalog.Get("ceph")
	require.True(t, ok)
	template, err := profile.Template()
	require.NoError(t, err)

	seen := make(map[string]bool, len(want))
	var walk func(group charttpl.Group)
	walk = func(group charttpl.Group) {
		for _, chart := range group.Charts {
			for _, dimension := range chart.Dimensions {
				expected, ok := want[dimension.Selector]
				if !ok {
					continue
				}
				seen[dimension.Selector] = true
				require.Equalf(t, expected.algorithm, chart.Algorithm, "selector %q algorithm", dimension.Selector)
				require.Equalf(t, expected.units, chart.Units, "selector %q units", dimension.Selector)
				if expected.divisor != 0 || expected.float {
					require.NotNilf(t, dimension.Options, "selector %q options", dimension.Selector)
					require.Equalf(t, expected.divisor, dimension.Options.Divisor, "selector %q divisor", dimension.Selector)
					require.Equalf(t, expected.float, dimension.Options.Float, "selector %q float mode", dimension.Selector)
				}
			}
		}
		for _, child := range group.Groups {
			walk(child)
		}
	}
	walk(template)

	for selector := range want {
		require.Truef(t, seen[selector], "source-lifecycle selector %q is not charted", selector)
	}
}

// TestDefaultCatalog_StockProfilesHaveMetadataDisposition keeps the runtime
// profile catalog and public integration catalog from drifting silently. A
// profile may point at Prometheus metadata or an equivalent first-class
// integration, but every stock profile must make that choice explicitly.
func TestDefaultCatalog_StockProfilesHaveMetadataDisposition(t *testing.T) {
	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)

	type disposition struct {
		metadataPath  string
		integrationID string
	}
	dispositions := map[string]disposition{
		"ceph": {
			metadataPath:  "../metadata.yaml",
			integrationID: "collector-go.d.plugin-prometheus-ceph",
		},
		"haproxy": {
			metadataPath:  "../../haproxy/metadata.yaml",
			integrationID: "collector-go.d.plugin-haproxy",
		},
		"litellm": {
			metadataPath:  "../metadata.yaml",
			integrationID: "collector-go.d.plugin-prometheus-litellm",
		},
		"vllm": {
			metadataPath:  "../metadata.yaml",
			integrationID: "collector-go.d.plugin-prometheus-vllm",
		},
		"vllm_ray": {
			metadataPath:  "../metadata.yaml",
			integrationID: "collector-go.d.plugin-prometheus-vllm",
		},
	}

	profiles := catalog.OrderedProfiles()
	profileNames := make(map[string]bool, len(profiles))
	for _, profile := range profiles {
		profileNames[profile.Name] = true
		_, ok := dispositions[profile.Name]
		require.Truef(t, ok, "stock profile %q must declare an integration metadata disposition", profile.Name)
	}

	for name, disposition := range dispositions {
		require.Truef(t, profileNames[name], "metadata disposition %q has no stock profile", name)
		content, err := os.ReadFile(disposition.metadataPath)
		require.NoErrorf(t, err, "read metadata disposition for stock profile %q", name)
		require.Containsf(t, string(content), "id: "+disposition.integrationID,
			"metadata disposition for stock profile %q must reference integration %q", name, disposition.integrationID)
	}
}
