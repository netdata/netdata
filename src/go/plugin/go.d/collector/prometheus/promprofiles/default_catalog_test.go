// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/proof"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/semantics"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/testutil"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
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

// TestDefaultCatalog_VLLMDropsEveryRayCompatibilityGauge derives Ray 2.48's
// unsuffixed compatibility gauges from the source-union fixture. The unified
// profile drops them before normalizing the surviving Ray families.
func TestDefaultCatalog_VLLMDropsEveryRayCompatibilityGauge(t *testing.T) {
	types := readPrometheusTypes(t, "prometheus/profiles/vllm/fixtures/vllm_ray_all_metrics.prom")

	var aliases []string
	for name, typ := range types {
		if typ == "gauge" && types[name+"_total"] == "counter" {
			aliases = append(aliases, name)
		}
	}
	slices.Sort(aliases)
	require.Len(t, aliases, 33)

	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)
	profile, ok := catalog.Get("vllm")
	require.True(t, ok)

	var sourceNames []string
	for name := range types {
		sourceNames = append(sourceNames, name)
	}
	var rayDrops []string
	for _, name := range profileRelabelDroppedNames(t, profile, sourceNames) {
		if strings.HasPrefix(name, "ray_vllm_") {
			rayDrops = append(rayDrops, name)
		}
	}
	require.Subset(t, rayDrops, aliases)
}

func TestDefaultCatalog_GeneratedEpochDropsMatchSourceUnion(t *testing.T) {
	tests := map[string]struct {
		fixture string
		prefix  string
	}{
		"litellm": {
			fixture: "prometheus/profiles/litellm/fixtures/litellm_all_metrics.prom", prefix: "litellm_",
		},
		"vllm": {fixture: "prometheus/profiles/vllm/fixtures/vllm_all_metrics.prom", prefix: "vllm:"},
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
			droppedCreated := profileRelabelDroppedNames(t, profile, sourceCreated)
			require.Equal(t, sourceCreated, droppedCreated,
				"stock profile generated-epoch drops must exactly track the external source union")
			if profileName == "litellm" {
				require.Empty(t, profileRelabelDroppedNames(t, profile, []string{
					"litellm_created_total",
					"other_created",
				}))
			}
		})
	}
}

func profileRelabelDroppedNames(t *testing.T, profile Profile, names []string) []string {
	t.Helper()

	blocks, err := profile.Relabeling()
	require.NoError(t, err)
	pipeline, err := relabel.NewPipeline(blocks)
	require.NoErrorf(t, err, "profile %q", profile.Name)
	var dropped []string
	for _, name := range names {
		_, drop := pipeline.Apply(prompkg.Sample{Name: name})
		if drop.Dropped() {
			dropped = append(dropped, name)
		}
	}
	slices.Sort(dropped)
	return dropped
}

func readPrometheusTypes(t *testing.T, relativePath string) map[string]string {
	t.Helper()

	file, err := os.Open(promtestutil.Require(t, relativePath))
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
	want := map[string][]string{
		"ceph":            {"ceph_netdata_future_metric"},
		"fastapi":         {"http_requests_netdata_future_metric"},
		"haproxy":         {"haproxy_netdata_future_metric"},
		"litellm":         {"litellm_netdata_future_metric"},
		"process_runtime": {"process_netdata_future_metric"},
		"python_gc":       {"python_gc_netdata_future_metric"},
		"vllm":            {"vllm:netdata_future_metric", "ray_vllm_netdata_future_metric"},
	}
	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)
	for _, profile := range catalog.OrderedProfiles() {
		name := profile.Name
		futureMetrics, ok := want[name]
		require.Truef(t, ok, "stock profile %q needs a forward-compatibility canary", name)
		scope, err := matcher.NewSimplePatternsMatcher(profile.Match)
		require.NoErrorf(t, err, "stock profile %q match must parse", name)
		selector := profile.AutogenSelector()
		if selector != nil {
			require.Emptyf(t, selector.Allow, "stock profile %q must not close fallback with an allowlist", name)
			for _, item := range selector.Deny {
				require.Truef(t, isExactPrometheusMetricName(item),
					"stock profile %q fallback deny %q must name one exact family", name, item)
			}
		}
		for _, futureMetric := range futureMetrics {
			require.Truef(t, scope.MatchString(futureMetric),
				"stock profile %q canary %q must be inside its match scope", name, futureMetric)
			if selector == nil {
				continue
			}
			compiled, err := selector.Parse()
			require.NoErrorf(t, err, "stock profile %q fallback selector must parse", name)
			require.Truef(t, compiled.Matches(futureMetric, nil),
				"stock profile %q must preserve unknown matching family %q", name, futureMetric)
		}
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

func TestDefaultCatalog_LiteLLMServiceTierRemainsOpenIdentity(t *testing.T) {
	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)
	profile, ok := catalog.Get("litellm")
	require.True(t, ok)
	template, err := profile.Template()
	require.NoError(t, err)

	tierIdentityCharts := 0
	var walk func(group charttpl.Group)
	walk = func(group charttpl.Group) {
		for _, chart := range group.Charts {
			if chart.Instances != nil && slices.Contains(chart.Instances.ByLabels, "service_tier") {
				tierIdentityCharts++
			}
			for _, dimension := range chart.Dimensions {
				require.NotContainsf(t, dimension.Selector, "{service_tier",
					"open service tier must not be converted into a finite selector classifier: %q", dimension.Selector)
				require.NotEqualf(t, "service_tier", dimension.NameFromLabel,
					"open service tier must remain NIDL identity, not a dynamic dimension: %q", dimension.Selector)
			}
		}
		for _, child := range group.Groups {
			walk(child)
		}
	}
	walk(template)
	require.Positive(t, tierIdentityCharts, "LiteLLM must retain service-tier identity on tier-sensitive views")
}

// TestDefaultCatalog_StockProfilesHaveMetadataDisposition keeps the runtime
// profile catalog and public integration catalog from drifting silently. A
// profile may point at Prometheus metadata or an equivalent first-class
// integration, but every stock profile must make that choice explicitly.
func TestDefaultCatalog_StockProfilesHaveMetadataDisposition(t *testing.T) {
	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)

	type disposition struct {
		metadataPath       string
		integrationID      string
		mustBeProofSupport bool
	}
	dispositions := map[string]disposition{
		"ceph": {
			metadataPath:  "../metadata.yaml",
			integrationID: "collector-go.d.plugin-prometheus-ceph",
		},
		"fastapi": {
			metadataPath:       "../metadata.yaml",
			integrationID:      "collector-go.d.plugin-prometheus-vllm",
			mustBeProofSupport: true,
		},
		"haproxy": {
			metadataPath:  "../../haproxy/metadata.yaml",
			integrationID: "collector-go.d.plugin-haproxy",
		},
		"litellm": {
			metadataPath:  "../metadata.yaml",
			integrationID: "collector-go.d.plugin-prometheus-litellm",
		},
		"process_runtime": {
			metadataPath:       "../metadata.yaml",
			integrationID:      "collector-go.d.plugin-prometheus-litellm",
			mustBeProofSupport: true,
		},
		"python_gc": {
			metadataPath:       "../metadata.yaml",
			integrationID:      "collector-go.d.plugin-prometheus-litellm",
			mustBeProofSupport: true,
		},
		"vllm": {
			metadataPath:  "../metadata.yaml",
			integrationID: "collector-go.d.plugin-prometheus-vllm",
		},
	}
	repoRoot, err := filepath.Abs("../../../../../../..")
	require.NoError(t, err)
	bundles, err := promproof.Discover(repoRoot)
	require.NoError(t, err)
	proofSupports := make(map[string]map[string]bool)
	for _, bundle := range bundles {
		identity := bundle.Descriptor.MetadataExample
		if identity == nil {
			continue
		}
		design, err := promsemantics.LoadProfileDesign(
			filepath.Join(repoRoot, filepath.FromSlash(bundle.ProfileDesignPath())),
		)
		require.NoError(t, err)
		if proofSupports[identity.IntegrationID] == nil {
			proofSupports[identity.IntegrationID] = make(map[string]bool)
		}
		for support := range design.Composition.Supports {
			proofSupports[identity.IntegrationID][support] = true
		}
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
		if disposition.mustBeProofSupport {
			require.Truef(t, proofSupports[disposition.integrationID][name],
				"metadata disposition for shared stock profile %q must be declared by an application proof for %q",
				name, disposition.integrationID)
		}
	}
}
