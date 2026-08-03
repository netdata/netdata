// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"bufio"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

var exactPrometheusMetricNamePattern = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)

func isExactPrometheusMetricName(value string) bool {
	return exactPrometheusMetricNamePattern.MatchString(strings.TrimSpace(value))
}

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
	types := readPrometheusTypes(t, "../testdata/vllm_ray_all_metrics.prom")

	var aliases []string
	for name, typ := range types {
		if typ == "gauge" && types[name+"_total"] == "counter" {
			aliases = append(aliases, name)
		}
	}
	slices.Sort(aliases)
	require.Len(t, aliases, 33)

	denials := append(aliases,
		"ray_vllm_kv_offload_size",
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

func TestDefaultCatalog_GeneratedEpochDenialsMatchSourceUnion(t *testing.T) {
	tests := map[string]struct {
		fixture string
		prefix  string
	}{
		"litellm": {fixture: "../testdata/litellm_all_metrics.prom", prefix: "litellm_"},
		"vllm":    {fixture: "../testdata/vllm_all_metrics.prom", prefix: "vllm:"},
	}

	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)
	for profileName, test := range tests {
		t.Run(profileName, func(t *testing.T) {
			var sourceCreated []string
			for name := range readPrometheusTypes(t, test.fixture) {
				if strings.HasPrefix(name, test.prefix) && strings.HasSuffix(name, "_created") {
					sourceCreated = append(sourceCreated, name)
				}
			}
			slices.Sort(sourceCreated)

			profile, ok := catalog.Get(profileName)
			require.True(t, ok)
			selector := profile.AutogenSelector()
			require.NotNil(t, selector)
			var deniedCreated []string
			for _, name := range selector.Deny {
				if strings.HasSuffix(name, "_created") {
					deniedCreated = append(deniedCreated, name)
				}
			}
			slices.Sort(deniedCreated)
			require.Equal(t, sourceCreated, deniedCreated,
				"stock profile generated-epoch denials must exactly track the committed source union")
		})
	}
}

func readPrometheusTypes(t *testing.T, path string) map[string]string {
	t.Helper()

	file, err := os.Open(path)
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
	return types
}

func TestDefaultCatalog_AllStockProfilesPreserveUnknownFutureFamilies(t *testing.T) {
	want := map[string]string{
		"ceph":     "ceph_netdata_future_metric",
		"haproxy":  "haproxy_netdata_future_metric",
		"litellm":  "litellm_netdata_future_metric",
		"vllm":     "vllm:netdata_future_metric",
		"vllm_ray": "ray_vllm_netdata_future_metric",
	}
	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)
	for _, profile := range catalog.OrderedProfiles() {
		name := profile.Name
		futureMetric, ok := want[name]
		require.Truef(t, ok, "stock profile %q needs a forward-compatibility canary", name)
		scope, err := matcher.NewSimplePatternsMatcher(profile.Match)
		require.NoErrorf(t, err, "stock profile %q match must parse", name)
		require.Truef(t, scope.MatchString(futureMetric),
			"stock profile %q canary %q must be inside its match scope", name, futureMetric)
		selector := profile.AutogenSelector()
		if selector == nil {
			continue
		}
		require.Emptyf(t, selector.Allow, "stock profile %q must not close fallback with an allowlist", name)
		for _, item := range selector.Deny {
			require.Truef(t, isExactPrometheusMetricName(item),
				"stock profile %q fallback deny %q must name one exact family", name, item)
		}
		compiled, err := selector.Parse()
		require.NoErrorf(t, err, "stock profile %q fallback selector must parse", name)
		require.Truef(t, compiled.Matches(futureMetric, nil),
			"stock profile %q must preserve unknown matching family %q", name, futureMetric)
	}
}

func TestDefaultCatalog_ExactFallbackDenySyntax(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "plain metric", value: "app_requests_total", want: true},
		{name: "colon metric", value: "vllm:requests_total", want: true},
		{name: "wildcard", value: "app_*", want: false},
		{name: "label constrained", value: `app_requests_total{tenant="a"}`, want: false},
		{name: "label only", value: `{tenant="a"}`, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, isExactPrometheusMetricName(test.value))
		})
	}
}

func TestDefaultCatalog_LiteLLMServiceTierClassifiersStayCanonical(t *testing.T) {
	const (
		valuePattern = `^(` +
			`[^N[:space:]][^[:space:]]*|N|N[^o[:space:]][^[:space:]]*|` +
			`No|No[^n[:space:]][^[:space:]]*|Non|Non[^e[:space:]][^[:space:]]*|None[^[:space:]]+` +
			`)$`
		positiveFilter          = `{service_tier=~"` + valuePattern + `"}`
		complementaryFilter     = `{service_tier!~"` + valuePattern + `"}`
		expectedClassifierPairs = 7
	)
	type pair struct {
		positive      int
		complementary int
	}

	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)
	profile, ok := catalog.Get("litellm")
	require.True(t, ok)
	template, err := profile.Template()
	require.NoError(t, err)

	pairs := make(map[string]pair)
	var walk func(group charttpl.Group)
	walk = func(group charttpl.Group) {
		for _, chart := range group.Charts {
			for _, dimension := range chart.Dimensions {
				if !strings.Contains(dimension.Selector, "service_tier") {
					continue
				}
				metric, filter, found := strings.Cut(dimension.Selector, "{")
				require.Truef(t, found, "service-tier selector %q has no label filter", dimension.Selector)
				filter = "{" + filter
				current := pairs[metric]
				switch filter {
				case positiveFilter:
					current.positive++
					require.Equalf(t, "service_tier", dimension.NameFromLabel,
						"positive service-tier selector %q must name the exported tier", dimension.Selector)
					require.Emptyf(t, dimension.Name,
						"positive service-tier selector %q must not use a fixed dimension", dimension.Selector)
				case complementaryFilter:
					current.complementary++
					require.Equalf(t, "unclassified", dimension.Name,
						"complementary service-tier selector %q must use the fallback dimension", dimension.Selector)
					require.Emptyf(t, dimension.NameFromLabel,
						"complementary service-tier selector %q must not read the absent/sentinel label", dimension.Selector)
				default:
					t.Fatalf("service-tier selector %q drifted from the canonical classifier", dimension.Selector)
				}
				pairs[metric] = current
			}
		}
		for _, child := range group.Groups {
			walk(child)
		}
	}
	walk(template)

	require.Len(t, pairs, expectedClassifierPairs)
	for metric, got := range pairs {
		require.Equalf(t, pair{positive: 1, complementary: 1}, got,
			"service-tier metric %q must have one positive and one complementary route", metric)
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
