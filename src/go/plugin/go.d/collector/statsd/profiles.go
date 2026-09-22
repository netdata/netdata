// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/pkg/relabel"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
)

const (
	profilesDirName = "statsd.profiles"
	// metricNameLabel is the relabel package's virtual label for Record.Name.
	metricNameLabel = "__name__"
)

// profileDocument is the strict profile file shape. The root match selects
// original StatsD names; relabeling and template are each optional, but a
// profile must provide at least one of them.
type profileDocument struct {
	Match      string          `yaml:"match"`
	Relabeling []relabel.Block `yaml:"relabeling,omitempty"`
	Template   *charttpl.Group `yaml:"template,omitempty"`
}

// profile is one prepared, configured profile. Its pipeline and input buffer
// are mutable and used only under the receiver lock.
type profile struct {
	name     string
	root     matcher.Matcher
	pipeline *relabel.Pipeline // nil for chart-only profiles
	groups   []charttpl.Group  // nil for replace-only profiles
	lifetime uint64            // longest chart or dimension expiry, in successful cycles
	input    []labels.Label    // reused per record and cleared, so it pins no earlier record
}

// defaultProfileDirs lists user configuration directories. There is no stock
// StatsD profile catalog; profiles are selected explicitly by name.
func defaultProfileDirs() []profilecatalog.DirSpec {
	var specs []profilecatalog.DirSpec
	for _, dir := range pluginconfig.CollectorsUserDirs() {
		specs = append(specs, profilecatalog.DirSpec{
			Path: filepath.Join(dir, profilesDirName),
		})
	}
	return specs
}

// loadProfiles resolves the configured names in order. Only selected files
// are decoded, so an unrelated invalid file cannot affect this job.
func loadProfiles(dirs []profilecatalog.DirSpec, names []string, log *logger.Logger) ([]*profile, error) {
	if len(names) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if !profilecatalog.DefaultValidName(name) {
			return nil, fmt.Errorf("profile name %q must match %s", name, profilecatalog.DefaultValidNamePattern)
		}
		if seen[name] {
			return nil, fmt.Errorf("profile %q is listed more than once", name)
		}
		seen[name] = true
	}
	catalog, err := profilecatalog.Load(dirs, profilecatalog.Options[string]{
		LoadFile: func(ctx profilecatalog.FileContext) (string, error) { return ctx.Path, nil },
		Log:      log,
	})
	if err != nil {
		return nil, fmt.Errorf("loading profiles: %w", err)
	}
	profiles := make([]*profile, 0, len(names))
	for _, name := range names {
		path, ok := catalog.Get(name)
		if !ok {
			return nil, fmt.Errorf("profile %q not found", name)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("profile %q: %w", name, err)
		}
		p, err := newProfile(name, data)
		if err != nil {
			return nil, fmt.Errorf("profile %q: %w", name, err)
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

func newProfile(name string, data []byte) (*profile, error) {
	var doc profileDocument
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	switch err := dec.Decode(new(yaml.Node)); {
	case errors.Is(err, io.EOF):
	case err != nil:
		return nil, err
	default:
		return nil, errors.New("a profile file must contain exactly one YAML document")
	}
	if strings.TrimSpace(doc.Match) == "" {
		return nil, errors.New("'match' is required")
	}
	root, err := matcher.NewSimplePatternsMatcher(doc.Match)
	if err != nil {
		return nil, fmt.Errorf("'match': %w", err)
	}
	if len(doc.Relabeling) == 0 && doc.Template == nil {
		return nil, errors.New("at least one of 'relabeling' or 'template' is required")
	}
	p := &profile{
		name: name,
		root: root,
	}
	for i, block := range doc.Relabeling {
		for j, rule := range block.MetricRelabelConfigs {
			if action := rule.WithDefaults().Action; action != relabel.Replace {
				return nil, fmt.Errorf(
					"relabeling[%d].metric_relabel_configs[%d]: action %q is not supported; only replace", i, j, action)
			}
		}
	}
	if p.pipeline, err = relabel.NewPipeline(doc.Relabeling); err != nil {
		return nil, err
	}
	if doc.Template != nil {
		group := *doc.Template
		if p.lifetime, err = resolveLifecycles(&group); err != nil {
			return nil, fmt.Errorf("'template': %w", err)
		}
		if p.lifetime == 0 {
			return nil, errors.New("'template' must contain at least one chart")
		}
		p.groups = []charttpl.Group{group}
		// Entry IDs namespace chart templates, so an entry valid alone is valid
		// with any other active entries.
		if _, err := chartengine.NewTemplateSet(chartengine.TemplateSetSpec{
			Entries:                  []chartengine.TemplateEntry{p.entry()},
			Policy:                   enginePolicy(),
			FallbackContextNamespace: contextNamespace,
		}); err != nil {
			return nil, fmt.Errorf("'template': %w", err)
		}
	}
	return p, nil
}

// resolveLifecycles supplies finite omitted chart and dimension expiry,
// preserves authored positive values and rejects a dimensions block that
// leaves expiry disabled. It returns the longest resolved expiry.
func resolveLifecycles(group *charttpl.Group) (uint64, error) {
	var longest int
	for i := range group.Charts {
		chart := &group.Charts[i]
		if chart.Lifecycle == nil {
			chart.Lifecycle = &charttpl.Lifecycle{}
		}
		lc := chart.Lifecycle
		if lc.ExpireAfterCycles == 0 {
			lc.ExpireAfterCycles = defaultChartExpiry
		}
		if lc.Dimensions == nil {
			lc.Dimensions = &charttpl.DimensionLifecycle{
				ExpireAfterCycles: defaultChartExpiry,
			}
		} else if lc.Dimensions.ExpireAfterCycles <= 0 {
			return 0, fmt.Errorf("chart %q: 'lifecycle.dimensions.expire_after_cycles' must be positive", chart.Context)
		}
		longest = max(longest, lc.ExpireAfterCycles, lc.Dimensions.ExpireAfterCycles)
	}
	for i := range group.Groups {
		nested, err := resolveLifecycles(&group.Groups[i])
		if err != nil {
			return 0, err
		}
		longest = max(longest, int(nested))
	}
	return uint64(max(longest, 0)), nil
}

func (p *profile) entry() chartengine.TemplateEntry {
	return chartengine.TemplateEntry{
		ID:               p.name,
		ContextNamespace: contextNamespace,
		Groups:           p.groups,
	}
}

// owns reports whether this profile's pipeline preprocesses an original name.
func (p *profile) owns(name string) bool {
	return p.pipeline != nil && p.root.MatchString(name) && p.pipeline.Matches(name)
}

// replace runs the ordered replace blocks on the name and labels only. The wire
// type, value, rate, gauge operation and member keep their meaning. Record.Labels
// must not contain the virtual __name__, so a sender label with that key stays
// outside the relabel record, invisible to rules, and is restored unchanged.
// Replaced labels are appended to labels[:0], so they may share the caller's storage.
//
// Label strings are substrings of the received datagram or buffered TCP records.
// The reused input buffer is cleared once the output is read, so the pipeline's
// processors keep only their results for the most recent input per block, a bound
// fixed by configuration and the record bound.
func (p *profile) replace(r record, labelsBuf []metrix.Label) (record, error) {
	var held *metrix.Label
	for i, l := range r.labels {
		if l.Key == metricNameLabel {
			held = &r.labels[i]
			continue
		}
		p.input = append(p.input, labels.Label{
			Name:  l.Key,
			Value: l.Value,
		})
	}
	// Record labels are sorted by key and unique, so the input is valid as it is.
	out, drop := p.pipeline.Apply(relabel.Record{
		Name:   r.name,
		Labels: labels.Labels(p.input),
	})
	replaced := labelsBuf[:0]
	if !drop.Dropped() {
		out.Labels.Range(func(l labels.Label) {
			replaced = append(replaced, metrix.Label{
				Key:   l.Name,
				Value: l.Value,
			})
		})
		if held != nil {
			replaced = append(replaced, *held)
		}
	}
	clear(p.input)
	p.input = p.input[:0]
	if drop.Dropped() {
		// Replace-only rules drop only when the resulting name is invalid.
		return r, rejectSyntax
	}
	r.name, r.labels = out.Name, replaced
	return r, nil
}
