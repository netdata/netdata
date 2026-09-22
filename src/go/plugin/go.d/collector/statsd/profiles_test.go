// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"
	"weak"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Profiles used by the receiver tests, in precedence order: pools owns
// svc.*.size names, shadow owns the remaining svc.* names, app renders from the
// shared final stream without preprocessing, and meta edits metadata and names.
var testProfiles = map[string]string{
	"pools": `
match: 'svc.*'
relabeling:
  - match: 'svc.*.size'
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: 'svc\.([^.]+)\.size'
        target_label: pool
        replacement: '$1'
      - source_labels: [__name__]
        regex: 'svc\.[^.]+\.size'
        target_label: __name__
        replacement: svc.pool.size
template:
  family: pools
  metrics:
    - g.value.svc.pool.size
  charts:
    - title: Pool size
      context: pools.size
      units: items
      instances:
        by_labels: [pool]
      dimensions:
        - selector: g.value.svc.pool.size
          name: size
`,
	"shadow": `
match: 'svc.*'
relabeling:
  - match: '*'
    metric_relabel_configs:
      - target_label: shadow
        replacement: 'yes'
`,
	"app": `
match: 'svc.* app.*'
template:
  family: app
  metrics:
    - c.total.app.requests
    - g.value.svc.pool.size
  charts:
    - title: Application requests
      context: app.requests
      units: requests/s
      dimensions:
        - selector: c.total.app.requests
          name: requests
    - title: Total pool size
      context: app.pool_size
      units: items
      dimensions:
        - selector: g.value.svc.pool.size
          name: size
`,
	"meta": `
match: 'meta.*'
relabeling:
  - match: 'meta.*'
    metric_relabel_configs:
      - target_label: nd_unit
        replacement: bytes
      - target_label: measure_field
        replacement: ''
      - source_labels: [__name__]
        regex: 'meta\.alias\..+'
        target_label: __name__
        replacement: meta.alias
      - source_labels: [__name__]
        regex: 'meta\.empty'
        target_label: __name__
        replacement: ''
      - source_labels: [__name__]
        regex: 'meta\.typo'
        target_label: nd_units
        replacement: bytes
`,
}

func writeProfiles(t testing.TB, files map[string]string) []profilecatalog.DirSpec {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(content), 0o600))
	}
	return []profilecatalog.DirSpec{{Path: dir}}
}

func newProfileFixture(t testing.TB, files map[string]string, names ...string) *coreFixture {
	t.Helper()
	c := New()
	c.Listeners = testListeners
	c.profileDirs = writeProfiles(t, files)
	c.Profiles = names
	f := prepareFixture(t, c)
	f.time = time.Unix(1000, 0)
	c.now = func() time.Time { return f.time }
	c.receiver.start()
	return f
}

func entryIDs(c *Collector) []string {
	var ids []string
	for _, e := range c.ChartTemplateSet().Entries() {
		ids = append(ids, e.ID)
	}
	return ids
}

func TestProfileConfigurationValidation(t *testing.T) {
	valid := testProfiles["app"]
	for name, tc := range map[string]struct {
		files    map[string]string
		profiles []string
		wantErr  string
	}{
		"unknown":            {map[string]string{"app": valid}, []string{"missing"}, `profile "missing" not found`},
		"invalid name":       {map[string]string{"app": valid}, []string{"App"}, "must match"},
		"listed twice":       {map[string]string{"app": valid}, []string{"app", "app"}, "more than once"},
		"unknown field":      {map[string]string{"p": "match: '*'\napp: x\n" + "relabeling: []\n"}, []string{"p"}, "field app not found"},
		"blank match":        {map[string]string{"p": "match: ' '\nrelabeling:\n  - match: '*'\n    metric_relabel_configs:\n      - target_label: a\n        replacement: b\n"}, []string{"p"}, "'match' is required"},
		"missing match":      {map[string]string{"p": "relabeling:\n  - match: '*'\n    metric_relabel_configs:\n      - target_label: a\n        replacement: b\n"}, []string{"p"}, "'match' is required"},
		"nothing to do":      {map[string]string{"p": "match: '*'\n"}, []string{"p"}, "at least one of"},
		"drop action":        {map[string]string{"p": "match: '*'\nrelabeling:\n  - match: '*'\n    metric_relabel_configs:\n      - action: drop\n        source_labels: [a]\n        regex: b\n"}, []string{"p"}, "only replace"},
		"lowercase action":   {map[string]string{"p": "match: '*'\nrelabeling:\n  - match: '*'\n    metric_relabel_configs:\n      - action: lowercase\n        source_labels: [a]\n        target_label: a\n"}, []string{"p"}, "only replace"},
		"invalid regex":      {map[string]string{"p": "match: '*'\nrelabeling:\n  - match: '*'\n    metric_relabel_configs:\n      - source_labels: [a]\n        regex: '('\n        target_label: b\n"}, []string{"p"}, "error parsing regexp"},
		"template no charts": {map[string]string{"p": "match: '*'\ntemplate:\n  family: x\n"}, []string{"p"}, "at least one chart"},
		"disabled dimension expiry": {map[string]string{"p": strings.Replace(valid, "      units: requests/s\n",
			"      units: requests/s\n      lifecycle:\n        dimensions:\n          max_dims: 3\n", 1)}, []string{"p"}, "must be positive"},
		"invalid template": {map[string]string{"p": "match: '*'\ntemplate:\n  metrics: [a]\n  charts:\n    - title: A\n      context: a\n      units: x\n      dimensions:\n        - selector: undeclared\n"}, []string{"p"}, "'template'"},
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.Listeners = testListeners
			c.profileDirs = writeProfiles(t, tc.files)
			c.Profiles = tc.profiles
			err := c.Init(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
	t.Run("unrelated invalid file does not affect the job", func(t *testing.T) {
		c := New()
		c.Listeners = testListeners
		c.profileDirs = writeProfiles(t, map[string]string{"app": valid, "broken": "{{{", "Bad-Name": valid})
		c.Profiles = []string{"app"}
		require.NoError(t, c.Init(context.Background()))
	})
}

func TestProfileLifetimes(t *testing.T) {
	authored := strings.Replace(testProfiles["app"], "      units: requests/s\n",
		"      units: requests/s\n      lifecycle:\n        expire_after_cycles: 30\n        dimensions:\n          expire_after_cycles: 12\n", 1)
	inherited := strings.Replace(testProfiles["app"], "      units: requests/s\n",
		"      units: requests/s\n      lifecycle:\n        dimensions:\n          expire_after_cycles: 7\n", 1)
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
	f.ingest(t,
		"svc.a.size:10|g", "svc.b.size:20|g", "svc.a.size:+1|g", // Delta keeps its operation after renaming.
		"svc.other:1|c|@.5",                                           // The second applicable owner runs; rate keeps its meaning.
		"svc.q.size:3|g|#__name__:sender",                             // A sender __name__ label is invisible to rules and kept.
		"meta.x:5|g|#measure_field:m,nd_unit:items",                   // Profile metadata overrides; reserved label removed.
		"meta.alias.a:10|g", "meta.alias.b:20|g", "meta.alias.a:+1|g", // Same-type aliases combine.
	)
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
	f := newProfileFixture(t, map[string]string{"jobs": `
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
	f.c.receiver.idle = time.Second
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
