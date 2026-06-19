// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

// A clean histogram plus an unrelated gauge sibling. Each corruption case relabels
// only the histogram; the sibling must always survive, proving the executor drops a
// single corrupted family without taking the rest of the scrape down.
const (
	relabelSibling = `
# TYPE sibling_gauge gauge
sibling_gauge{x="1"} 7
`
	relabelHistogram = `
# HELP app_lat Request latency.
# TYPE app_lat histogram
app_lat_bucket{le="0.1"} 4
app_lat_bucket{le="0.2"} 5
app_lat_bucket{le="+Inf"} 6
app_lat_sum 2.5
app_lat_count 6
` + relabelSibling

	relabelSummary = `
# TYPE app_q summary
app_q{quantile="0.5"} 0.25
app_q{quantile="0.9"} 0.5
app_q_sum 12.5
app_q_count 42
` + relabelSibling

	// Two histogram instances distinguished only by the inst label; dropping inst
	// merges them into one final family.
	relabelTwoHistograms = `
# TYPE app_lat histogram
app_lat_bucket{inst="a",le="0.1"} 4
app_lat_bucket{inst="a",le="+Inf"} 6
app_lat_sum{inst="a"} 2.5
app_lat_count{inst="a"} 6
app_lat_bucket{inst="b",le="0.1"} 3
app_lat_bucket{inst="b",le="+Inf"} 5
app_lat_sum{inst="b"} 1.5
app_lat_count{inst="b"} 5
` + relabelSibling
)

// TestCollector_relabelTypedFamilyIntegrity drives the real collector (Init → Check,
// Init → Collect) and asserts that relabeling which would silently corrupt a
// histogram/summary is a hard error under Check and a drop-and-reassemble under
// Collect, while clean relabeling and the unrelated sibling are preserved.
func TestCollector_relabelTypedFamilyIntegrity(t *testing.T) {
	tests := map[string]struct {
		input        string
		rules        []relabel.Config
		wantCheckErr bool
		assert       func(t *testing.T, fr metrix.Reader)
	}{
		"clean full rename is allowed": {
			input:        relabelHistogram,
			rules:        renameMetric("app_lat(.*)", "renamed_lat${1}"),
			wantCheckErr: false,
			assert: func(t *testing.T, fr metrix.Reader) {
				assert.InDelta(t, 4, value(t, fr, "renamed_lat_bucket", metrix.Labels{"le": "0.1"}), 1e-9)
				assert.InDelta(t, 2.5, value(t, fr, "renamed_lat_sum", nil), 1e-9)
				assert.InDelta(t, 6, value(t, fr, "renamed_lat_count", nil), 1e-9)
				noSeries(t, fr, "app_lat_bucket", metrix.Labels{"le": "0.1"})
				assert.InDelta(t, 7, value(t, fr, "sibling_gauge", metrix.Labels{"x": "1"}), 1e-9)
			},
		},
		"clean rename of a multi-instance histogram is allowed": {
			input:        relabelTwoHistograms,
			rules:        renameMetric("app_lat(.*)", "renamed_lat${1}"),
			wantCheckErr: false,
			assert: func(t *testing.T, fr metrix.Reader) {
				// Both instances rename consistently and keep their distinguishing
				// label, so they stay distinct families — no false split/merge.
				assert.InDelta(t, 4, value(t, fr, "renamed_lat_bucket", metrix.Labels{"inst": "a", "le": "0.1"}), 1e-9)
				assert.InDelta(t, 1.5, value(t, fr, "renamed_lat_sum", metrix.Labels{"inst": "b"}), 1e-9)
				noSeries(t, fr, "app_lat_bucket", metrix.Labels{"inst": "a", "le": "0.1"})
				assert.InDelta(t, 7, value(t, fr, "sibling_gauge", metrix.Labels{"x": "1"}), 1e-9)
			},
		},
		"non-matching rules leave the scrape intact": {
			input:        relabelHistogram,
			rules:        renameMetric("does_not_exist", "x"),
			wantCheckErr: false,
			assert: func(t *testing.T, fr metrix.Reader) {
				assert.InDelta(t, 4, value(t, fr, "app_lat_bucket", metrix.Labels{"le": "0.1"}), 1e-9)
				assert.InDelta(t, 2.5, value(t, fr, "app_lat_sum", nil), 1e-9)
				assert.InDelta(t, 7, value(t, fr, "sibling_gauge", metrix.Labels{"x": "1"}), 1e-9)
			},
		},
		"renaming only _sum splits the histogram": {
			input:        relabelHistogram,
			rules:        renameMetric("app_lat_sum", "other_lat_sum"),
			wantCheckErr: true,
			assert: func(t *testing.T, fr metrix.Reader) {
				// The whole family is dropped — app_lat is NOT written with a fabricated sum=0,
				// and the orphaned other_lat_sum is dropped too.
				noSeries(t, fr, "app_lat_bucket", metrix.Labels{"le": "0.1"})
				noSeries(t, fr, "app_lat_sum", nil)
				noSeries(t, fr, "app_lat_count", nil)
				noSeries(t, fr, "other_lat_sum", nil)
				assert.InDelta(t, 7, value(t, fr, "sibling_gauge", metrix.Labels{"x": "1"}), 1e-9)
			},
		},
		"dropping only _sum is a partial drop": {
			input:        relabelHistogram,
			rules:        dropMetric("app_lat_sum"),
			wantCheckErr: true,
			assert: func(t *testing.T, fr metrix.Reader) {
				noSeries(t, fr, "app_lat_bucket", metrix.Labels{"le": "0.1"})
				noSeries(t, fr, "app_lat_count", nil)
				assert.InDelta(t, 7, value(t, fr, "sibling_gauge", metrix.Labels{"x": "1"}), 1e-9)
			},
		},
		"mutating a bucket le corrupts the histogram": {
			input:        relabelHistogram,
			rules:        setLabel([]string{"le"}, "0.1", "le", "0.15"),
			wantCheckErr: true,
			assert: func(t *testing.T, fr metrix.Reader) {
				noSeries(t, fr, "app_lat_bucket", metrix.Labels{"le": "0.15"})
				noSeries(t, fr, "app_lat_sum", nil)
				assert.InDelta(t, 7, value(t, fr, "sibling_gauge", metrix.Labels{"x": "1"}), 1e-9)
			},
		},
		"mutating a summary quantile corrupts the summary": {
			input:        relabelSummary,
			rules:        setLabel([]string{"quantile"}, "0.5", "quantile", "0.7"),
			wantCheckErr: true,
			assert: func(t *testing.T, fr metrix.Reader) {
				noSeries(t, fr, "app_q", metrix.Labels{"quantile": "0.7"})
				noSeries(t, fr, "app_q_sum", nil)
				assert.InDelta(t, 7, value(t, fr, "sibling_gauge", metrix.Labels{"x": "1"}), 1e-9)
			},
		},
		"dropping a distinguishing label merges two histograms": {
			input:        relabelTwoHistograms,
			rules:        dropLabel("inst"),
			wantCheckErr: true,
			assert: func(t *testing.T, fr metrix.Reader) {
				noSeries(t, fr, "app_lat_sum", nil)
				noSeries(t, fr, "app_lat_count", nil)
				assert.InDelta(t, 7, value(t, fr, "sibling_gauge", metrix.Labels{"x": "1"}), 1e-9)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tc.input)) }))
			defer srv.Close()

			newCollr := func() *Collector {
				c := New()
				c.URL = srv.URL
				c.Relabeling = []RelabelBlock{{Match: "*", MetricRelabelConfigs: tc.rules}}
				require.NoError(t, c.Init(context.Background()))
				return c
			}

			// Check: corruption fails autodetection.
			checkErr := newCollr().Check(context.Background())
			if tc.wantCheckErr {
				assert.Error(t, checkErr)
			} else {
				assert.NoError(t, checkErr)
			}

			// Collect: corruption is dropped, survivors written.
			collr := newCollr()
			cc := cycle(t, collr.MetricStore())
			cc.BeginCycle()
			require.NoError(t, collr.Collect(context.Background()))
			require.NoError(t, cc.CommitCycleSuccess())

			tc.assert(t, collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
		})
	}
}

// TestCollector_relabelHelpRemap verifies HELP follows a family rename so the
// renamed family keeps its description.
func TestCollector_relabelHelpRemap(t *testing.T) {
	batch := scrapeSamples(t, `
# HELP app_lat Request latency.
# TYPE app_lat histogram
app_lat_bucket{le="+Inf"} 6
app_lat_sum 2.5
app_lat_count 6
`)

	proc, err := relabel.New(renameMetric("app_lat(.*)", "renamed_lat${1}"))
	require.NoError(t, err)
	c := &Collector{relabelBlocks: []relabelBlock{{match: matcher.TRUE(), proc: proc}}}

	mfs, err := c.relabelAndAssemble(batch, false)
	require.NoError(t, err)

	renamed := mfs.GetHistogram("renamed_lat")
	require.NotNil(t, renamed)
	assert.Equal(t, "Request latency.", renamed.Help())
	assert.Nil(t, mfs.Get("app_lat"))
}

// TestCollector_relabelBlockMatch verifies a block's match scopes its rules to the
// matching metric-name subset, leaving non-matching metrics untouched.
func TestCollector_relabelBlockMatch(t *testing.T) {
	input := `
# TYPE app_requests_total counter
app_requests_total{code="200"} 5
# TYPE other_requests_total counter
other_requests_total{code="200"} 9
`
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(input)) }))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	collr.Relabeling = []RelabelBlock{{
		Match:                "app_*",
		MetricRelabelConfigs: setLabel([]string{"code"}, "(.+)", "verb", "${1}"),
	}}
	require.NoError(t, collr.Init(context.Background()))

	cc := cycle(t, collr.MetricStore())
	cc.BeginCycle()
	require.NoError(t, collr.Collect(context.Background()))
	require.NoError(t, cc.CommitCycleSuccess())

	fr := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	// app_* matched: code copied to verb.
	assert.InDelta(t, 5, value(t, fr, "app_requests_total", metrix.Labels{"code": "200", "verb": "200"}), 1e-9)
	// other_* not matched: untouched, no verb label.
	assert.InDelta(t, 9, value(t, fr, "other_requests_total", metrix.Labels{"code": "200"}), 1e-9)
	noSeries(t, fr, "other_requests_total", metrix.Labels{"code": "200", "verb": "200"})
}

// TestValidateTypedFamilies_backstops covers the two safety-net branches that the
// reachable end-to-end cases shadow: a duplicate component the assembler hides
// (last-write-wins on _sum/_count), and a touched family whose final key does not
// assemble into a valid typed family.
func TestValidateTypedFamilies_backstops(t *testing.T) {
	batch := scrapeSamples(t, `
# TYPE app_lat histogram
app_lat_bucket{le="+Inf"} 6
app_lat_sum 2.5
app_lat_count 6
`)
	mfs, err := prompkg.Assemble(batch)
	require.NoError(t, err)

	var key typedFamilyKey
	for _, s := range batch.Samples {
		if s.Kind == prompkg.SampleKindHistogramSum {
			key, _ = typedFamilyKeyOf(s)
		}
	}

	t.Run("duplicate component is rejected", func(t *testing.T) {
		tr := newRelabelTracking()
		tr.anyTypedTouched = true
		rs := tr.rawState(key)
		rs.touched = true
		rs.rawCount, rs.keptCount = 3, 3
		rs.finalKeys[key] = struct{}{}
		fs := tr.finalState(key)
		fs.touched = true
		fs.rawKeys[key] = struct{}{}
		fs.sumCount = 2 // two _sum samples folded into one final family

		invalid, violations := validateTypedFamilies(tr, mfs)
		_, bad := invalid[key]
		assert.True(t, bad, "a duplicate component must invalidate the family")
		assert.NotEmpty(t, violations)
	})

	t.Run("family that does not assemble is rejected", func(t *testing.T) {
		missing := typedFamilyKey{name: "ghost", hash: 1234}
		tr := newRelabelTracking()
		tr.anyTypedTouched = true
		rs := tr.rawState(missing)
		rs.touched = true
		rs.rawCount, rs.keptCount = 1, 1
		rs.finalKeys[missing] = struct{}{}

		invalid, violations := validateTypedFamilies(tr, mfs) // mfs has app_lat, not ghost
		_, bad := invalid[missing]
		assert.True(t, bad, "a family absent from the assembled output must be invalidated")
		assert.NotEmpty(t, violations)
	})
}

func scrapeSamples(tb testing.TB, exposition string) prompkg.SampleBatch {
	tb.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(exposition)) }))
	tb.Cleanup(srv.Close)

	batch, err := prompkg.New(srv.Client(), web.RequestConfig{URL: srv.URL}).ScrapeSamples(context.Background())
	require.NoError(tb, err)
	return batch
}

func noSeries(t *testing.T, fr metrix.Reader, name string, labels metrix.Labels) {
	t.Helper()
	_, ok := fr.Value(name, labels)
	assert.Falsef(t, ok, "expected series %q labels=%v to be absent", name, labels)
}

func renameMetric(regex, replacement string) []relabel.Config {
	return []relabel.Config{{
		SourceLabels: []string{"__name__"},
		Regex:        relabel.MustNewRegexp(regex),
		TargetLabel:  "__name__",
		Replacement:  replacement,
		Action:       relabel.Replace,
	}}
}

func dropMetric(regex string) []relabel.Config {
	return []relabel.Config{{
		SourceLabels: []string{"__name__"},
		Regex:        relabel.MustNewRegexp(regex),
		Action:       relabel.Drop,
	}}
}

func dropLabel(regex string) []relabel.Config {
	return []relabel.Config{{
		Regex:  relabel.MustNewRegexp(regex),
		Action: relabel.LabelDrop,
	}}
}

func setLabel(sourceLabels []string, regex, target, replacement string) []relabel.Config {
	return []relabel.Config{{
		SourceLabels: sourceLabels,
		Regex:        relabel.MustNewRegexp(regex),
		TargetLabel:  target,
		Replacement:  replacement,
		Action:       relabel.Replace,
	}}
}
