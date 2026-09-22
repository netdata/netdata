// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"
	"weak"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initProfiles runs Init with the given profile files and selected names.
func initProfiles(t *testing.T, files map[string]string, names ...string) error {
	t.Helper()
	c := New()
	c.Listeners = testListeners
	c.profileDirs = writeProfiles(t, files)
	c.Profiles = names
	return c.Init(context.Background())
}

// appWithLifecycle adds a lifecycle block to the app profile's requests chart.
func appWithLifecycle(lifecycle string) string {
	return strings.Replace(testProfiles["app"], "      units: requests/s\n",
		"      units: requests/s\n      lifecycle:\n"+strings.TrimPrefix(lifecycle, "\n")+"\n", 1)
}

func TestProfileSelectionValidation(t *testing.T) {
	files := map[string]string{"app": testProfiles["app"], "broken": "{{{", "Bad-Name": testProfiles["app"]}
	for name, tc := range map[string]struct {
		names   []string
		wantErr string
	}{
		"unknown":      {names: []string{"missing"}, wantErr: `profile "missing" not found`},
		"invalid name": {names: []string{"App"}, wantErr: "must match"},
		"listed twice": {names: []string{"app", "app"}, wantErr: "more than once"},
		// Only selected files are decoded, so unrelated invalid files do not matter.
		"unrelated invalid files": {names: []string{"app"}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := initProfiles(t, files, tc.names...); tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}

func TestProfileDocumentValidation(t *testing.T) {
	const rule = `
relabeling:
  - match: '*'
    metric_relabel_configs:
      - target_label: a
        replacement: b
`
	for name, tc := range map[string]struct {
		doc, wantErr string
	}{
		"unknown field":      {doc: "match: '*'\napp: x" + rule, wantErr: "field app not found"},
		"blank match":        {doc: "match: ' '" + rule, wantErr: "'match' is required"},
		"missing match":      {doc: rule, wantErr: "'match' is required"},
		"nothing to do":      {doc: "match: '*'", wantErr: "at least one of"},
		"template no charts": {doc: "match: '*'\ntemplate:\n  family: x", wantErr: "at least one chart"},
		"drop action": {doc: `
match: '*'
relabeling:
  - match: '*'
    metric_relabel_configs:
      - action: drop
        source_labels: [a]
        regex: b
`, wantErr: "only replace"},
		"lowercase action": {doc: `
match: '*'
relabeling:
  - match: '*'
    metric_relabel_configs:
      - action: lowercase
        source_labels: [a]
        target_label: a
`, wantErr: "only replace"},
		"invalid regex": {doc: `
match: '*'
relabeling:
  - match: '*'
    metric_relabel_configs:
      - source_labels: [a]
        regex: '('
        target_label: b
`, wantErr: "error parsing regexp"},
		"invalid template": {doc: `
match: '*'
template:
  metrics: [a]
  charts:
    - title: A
      context: a
      units: x
      dimensions:
        - selector: undeclared
`, wantErr: "'template'"},
		"misspelled rule field": {doc: `
match: '*'
relabeling:
  - match: '*'
    metric_relabel_configs:
      - target_lable: a
        replacement: b
`, wantErr: "field target_lable not found"},
		"second document": {
			doc:     testProfiles["app"] + "---\nmatch: 'other.*'\n",
			wantErr: "exactly one YAML document",
		},
		"malformed trailing document": {doc: testProfiles["app"] + "---\nmatch: [\n", wantErr: "yaml"},
		"disabled dimension expiry": {doc: appWithLifecycle(`
        dimensions:
          max_dims: 3`), wantErr: "must be positive"},
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, initProfiles(t, map[string]string{"p": tc.doc}, "p"), tc.wantErr)
		})
	}
}

func TestProfileLifetimes(t *testing.T) {
	authored := appWithLifecycle(`
        expire_after_cycles: 30
        dimensions:
          expire_after_cycles: 12`)
	inherited := appWithLifecycle(`
        dimensions:
          expire_after_cycles: 7`)
	for name, tc := range map[string]struct {
		profile string
		want    []charttpl.Lifecycle
		horizon uint64
	}{
		// Omitted lifetimes become finite; metrix retention (10+10) dominates the horizon.
		"omitted": {testProfiles["app"], []charttpl.Lifecycle{
			{ExpireAfterCycles: 5, Dimensions: &charttpl.DimensionLifecycle{
				ExpireAfterCycles: 5,
			}},
			{ExpireAfterCycles: 5, Dimensions: &charttpl.DimensionLifecycle{
				ExpireAfterCycles: 5,
			}},
		}, 20},
		// Authored positive values are preserved and extend metadata authority.
		"authored": {authored, []charttpl.Lifecycle{
			{ExpireAfterCycles: 30, Dimensions: &charttpl.DimensionLifecycle{
				ExpireAfterCycles: 12,
			}},
			{ExpireAfterCycles: 5, Dimensions: &charttpl.DimensionLifecycle{
				ExpireAfterCycles: 5,
			}},
		}, 31},
		// Chart expiry zero inherits the native default; it is not a disabled sentinel.
		"inherited chart expiry": {inherited, []charttpl.Lifecycle{
			{ExpireAfterCycles: 5, Dimensions: &charttpl.DimensionLifecycle{
				ExpireAfterCycles: 7,
			}},
			{ExpireAfterCycles: 5, Dimensions: &charttpl.DimensionLifecycle{
				ExpireAfterCycles: 5,
			}},
		}, 20},
	} {
		t.Run(name, func(t *testing.T) {
			f := newProfileFixture(t, map[string]string{"app": tc.profile}, "app")
			var got []charttpl.Lifecycle
			for _, chart := range f.c.profiles[0].groups[0].Charts {
				got = append(got, *chart.Lifecycle)
			}
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.horizon, f.c.receiver.horizon)
		})
	}
}

func TestProfileReplaceAndIdentity(t *testing.T) {
	f := newProfileFixture(t, testProfiles, "pools", "shadow", "app", "meta")
	// A delta keeps its operation after renaming.
	f.ingest(t, "svc.a.size:10|g", "svc.b.size:20|g", "svc.a.size:+1|g")
	// The second applicable owner runs; the rate keeps its meaning.
	f.ingest(t, "svc.other:1|c|@.5")
	// A sender __name__ label is invisible to rules and kept.
	f.ingest(t, "svc.q.size:3|g|#__name__:sender")
	// Profile metadata overrides the sender's; the reserved label is removed.
	f.ingest(t, "meta.x:5|g|#measure_field:m,nd_unit:items")
	// Same-type aliases combine.
	f.ingest(t, "meta.alias.a:10|g", "meta.alias.b:20|g", "meta.alias.a:+1|g")
	// Rules can source sender tags.
	f.ingest(t, "meta.tagged:1|g|#zone:eu")
	for line, want := range map[string]rejection{
		"svc.c.size:1|c": rejectType,     // Type binding follows the final name.
		"meta.empty:1|g": rejectSyntax,   // Replacement produced an invalid name.
		"meta.typo:1|g":  rejectMetadata, // Unknown nd_ keys reject after replacement.
	} {
		require.ErrorIs(t, f.c.receiver.ingest(line, f.time), want, line)
	}
	f.collect(t, false, false)
	value(t, f.c, "g.value.svc.pool.size", 11, metrix.Labels{
		"pool": "a",
	})
	value(t, f.c, "g.value.svc.pool.size", 20, metrix.Labels{
		"pool": "b",
	})
	value(t, f.c, "g.value.svc.pool.size", 3, metrix.Labels{
		"pool":     "q",
		"__name__": "sender",
	})
	value(t, f.c, "c.total.svc.other", 2, metrix.Labels{
		"shadow": "yes",
	})
	value(t, f.c, "g.value.meta.x", 5, nil)
	value(t, f.c, "g.value.meta.alias", 21, nil)
	value(t, f.c, "g.value.meta.tagged", 1, metrix.Labels{"zone": "eu", "region": "r-eu"})
	meta, ok := f.c.store.Read().MetricMeta("g.value.meta.x")
	require.True(t, ok)
	assert.Equal(t, "bytes", meta.Unit)
	var names []string
	f.c.store.Read(metrix.ReadRaw()).ForEachSeries(func(name string, labels metrix.LabelView, _ metrix.SampleValue) {
		if _, shadowed := labels.Get("shadow"); shadowed && strings.HasPrefix(name, "g.value.svc.pool") {
			names = append(names, name)
		}
	})
	assert.Empty(t, names, "only the first applicable profile preprocesses an input")
}

func TestProfileActivation(t *testing.T) {
	f := newProfileFixture(t, testProfiles, "pools", "shadow", "app", "meta")
	initial := f.c.ChartTemplateSet()
	assert.Equal(t, []string{diagnosticsEntryID}, entryIDs(f.c), "eligible profiles are not preloaded")

	// Matching input that is not fully admitted activates nothing.
	require.ErrorIs(t, f.c.receiver.ingest("svc.a.size:x|g", f.time), rejectValue)
	require.ErrorIs(t, f.c.receiver.ingest("svc.a.size:+1|g", f.time), rejectBaseline)
	f.collect(t, false, false)
	assert.Same(t, initial, f.c.ChartTemplateSet())

	// The first admitted input activates every matching profile with templates,
	// and its value is published with the new charts in the same cycle.
	f.ingest(t, "svc.a.size:10|g")
	wire := f.collect(t, false, false)
	assert.Equal(t, []string{diagnosticsEntryID, "pools", "app"}, entryIDs(f.c))
	assert.Contains(t, wire, "'statsd.pools.size'")
	assert.Contains(t, wire, "'statsd.app.pool_size'")
	assert.Contains(t, wire, "CLABEL 'pool' 'a'")
	activated := f.c.ChartTemplateSet()
	assert.NotSame(t, initial, activated)

	// Unchanged membership reuses the pointer; silence never deactivates.
	for range 30 {
		f.time = f.time.Add(time.Second)
		f.collect(t, false, false)
	}
	assert.Same(t, activated, f.c.ChartTemplateSet())

	// A replace-only profile never becomes a native entry.
	f.ingest(t, "meta.x:1|g")
	f.collect(t, false, false)
	assert.Same(t, activated, f.c.ChartTemplateSet())
}

func TestActivationIsCapturedWithItsBatch(t *testing.T) {
	f := newProfileFixture(t, testProfiles, "pools", "app")
	first, err := f.c.receiver.cut(f.time, 0)
	require.NoError(t, err)
	assert.Nil(t, first.membership)
	// Input admitted while the previous batch is detached belongs to the next cut.
	f.ingest(t, "app.requests:3|c")
	f.c.receiver.release(first.batch)

	second, err := f.c.receiver.cut(f.time, 0)
	require.NoError(t, err)
	assert.Equal(t, []bool{false, true}, second.membership)
	require.Len(t, second.batch, 1)
	assert.Equal(t, 3.0, second.batch[0].value)
	f.c.receiver.release(second.batch)

	third, err := f.c.receiver.cut(f.time, second.activated)
	require.NoError(t, err)
	assert.Nil(t, third.membership, "membership already published")
	f.c.receiver.release(third.batch)
}

func TestActivationSurvivesPublicationFailure(t *testing.T) {
	for name, metricAbort := range map[string]bool{"metric abort": true, "publication abort": false} {
		t.Run(name, func(t *testing.T) {
			f := newProfileFixture(t, testProfiles, "app")
			f.ingest(t, "app.requests:3|c", "app.latency:10|ms")
			f.collect(t, metricAbort, !metricAbort)
			// The failed cycle's interval is lost (no replay), but activation is kept:
			// the next successful publication creates the chart with the held total.
			wire := f.collect(t, false, false)
			assert.Contains(t, wire, "'statsd.app.requests'")
			value(t, f.c, "c.total.app.requests", 3, nil)
			value(t, f.c, "ms.count.app.latency", 0, nil)
		})
	}
}

// TestProfileDimensionChurnRetires uses the case that motivates finite omitted
// dimension expiry: a name_from_label chart whose label values keep changing.
func TestProfileDimensionChurnRetires(t *testing.T) {
	idle := func(c *Collector) { c.MetricIdleTimeout = confopt.Duration(time.Second) }
	f := newConfiguredProfileFixture(t, idle, map[string]string{"jobs": `
match: 'jobs.*'
template:
  family: jobs
  metrics: [g.value.jobs.active]
  charts:
    - title: Active jobs
      context: jobs.active
      units: jobs
      dimensions:
        - selector: g.value.jobs.active
          name_from_label: worker
`}, "jobs")
	live := map[string]bool{}
	chartLive, created, peak := false, 0, 0
	track := func(wire string) {
		inChart := false
		for line := range strings.Lines(wire) {
			switch {
			case strings.HasPrefix(line, "CHART "):
				inChart = strings.Contains(line, "'statsd.jobs.active'")
				if inChart {
					chartLive = !strings.Contains(line, "obsolete")
					if !chartLive {
						clear(live) // An obsolete chart retires all its dimensions.
					}
				}
			case inChart && strings.HasPrefix(line, "DIMENSION "):
				id := strings.Split(line, "'")[1]
				if strings.Contains(line, "obsolete") {
					delete(live, id)
				} else if !live[id] {
					live[id] = true
					created++
				}
			}
		}
		peak = max(peak, len(live))
	}
	const generations = 40
	for g := range generations {
		f.ingest(t, fmt.Sprintf("jobs.active:1|g|#worker:w%d", g))
		f.time = f.time.Add(time.Second)
		track(f.collect(t, false, false))
	}
	for range int(f.c.receiver.horizon) + 2 {
		f.time = f.time.Add(time.Second)
		track(f.collect(t, false, false))
	}
	assert.Equal(t, generations, created)
	assert.LessOrEqual(t, peak, defaultChartExpiry+2, "expired dimensions retire")
	assert.Empty(t, live)
	assert.False(t, chartLive, "the idle chart retires")
	assert.Empty(t, f.c.receiver.entries)
	assert.Empty(t, f.c.receiver.metadata)
	assert.Zero(t, applicationSeries(f.c))
}

// TestProfileReplaceRetainsNoInputRecord: the owning profile's reusable state
// must not keep a large earlier record alive once a smaller one follows.
func TestProfileReplaceRetainsNoInputRecord(t *testing.T) {
	for name, key := range map[string]string{"admitted": "t", "rejected": "t!"} {
		t.Run(name, func(t *testing.T) {
			f := newProfileFixture(t, testProfiles, "shadow")
			large := func() weak.Pointer[byte] {
				var b strings.Builder
				b.WriteString("svc.large:1|c|#")
				for i := range 400 {
					if i > 0 {
						b.WriteByte(',')
					}
					fmt.Fprintf(&b, "%s%d:%s", key, i, strings.Repeat("v", 100))
				}
				line := b.String()
				_ = f.c.receiver.ingest(line, f.time)
				return weak.Make(unsafe.StringData(line))
			}()
			f.ingest(t, "svc.small:1|c|#a:b")
			runtime.GC()
			runtime.GC()
			assert.Nil(t, large.Value(), "a previous input record is still reachable")
		})
	}
}
