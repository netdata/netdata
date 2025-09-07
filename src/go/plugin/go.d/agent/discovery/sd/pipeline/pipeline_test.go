// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"

	"github.com/goccy/go-yaml"
	"github.com/gohugoio/hashstructure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_defaultConfigs(t *testing.T) {
	dir := "../../../../config/go.d/sd/"
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	require.NotEmpty(t, entries)

	for _, e := range entries {
		if strings.Contains(e.Name(), "prometheus") {
			continue
		}
		file, err := filepath.Abs(filepath.Join(dir, e.Name()))
		require.NoError(t, err, "abs path")

		bs, err := os.ReadFile(file)
		require.NoErrorf(t, err, "read config file '%s'", file)

		var cfg Config
		require.NoErrorf(t, yaml.Unmarshal(bs, &cfg), "unmarshal '%s'", e.Name())

		_, err = New(cfg)
		require.NoErrorf(t, err, "create pipeline '%s'", e.Name())
	}
}

func TestNew(t *testing.T) {
	tests := map[string]struct {
		config  string
		wantErr bool
	}{
		"fails when config unset": {
			wantErr: true,
			config:  "",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {

			var cfg Config
			err := yaml.Unmarshal([]byte(test.config), &cfg)
			require.Nilf(t, err, "cfg unmarshal")

			_, err = New(cfg)

			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPipeline_Run(t *testing.T) {
	const config = `
classify:
  - selector: "rule1"
    tags: "foo1"
    match:
      - tags: "bar1"
        expr: '{{ glob .Name "mock*1*" }}'
      - tags: "bar2"
        expr: '{{ glob .Name "mock*2*" }}'
compose:
  - selector: "foo1"
    config:
      - selector: "bar1"
        template: |
          name: {{ .Name }}-foobar1
      - selector: "bar2"
        template: |
          name: {{ .Name }}-foobar2
`
	tests := map[string]discoverySim{
		"new group with no targets": {
			config: config,
			discoverers: []model.Discoverer{
				newMockDiscoverer("",
					newMockTargetGroup("test"),
				),
			},
			wantClassifyCalls: 0,
			wantComposeCalls:  0,
			wantConfGroups:    nil,
		},
		"new group with targets": {
			config: config,
			discoverers: []model.Discoverer{
				newMockDiscoverer("rule1",
					newMockTargetGroup("test", "mock1", "mock2"),
				),
			},
			wantClassifyCalls: 2,
			wantComposeCalls:  2,
			wantConfGroups: []*confgroup.Group{
				prepareDiscoveredGroup("mock1-foobar1", "mock2-foobar2"),
			},
		},
		"existing group with same targets": {
			config: config,
			discoverers: []model.Discoverer{
				newMockDiscoverer("rule1",
					newMockTargetGroup("test", "mock1", "mock2"),
				),
				newDelayedMockDiscoverer("rule1", 5,
					newMockTargetGroup("test", "mock1", "mock2"),
				),
			},
			wantClassifyCalls: 2,
			wantComposeCalls:  2,
			wantConfGroups: []*confgroup.Group{
				prepareDiscoveredGroup("mock1-foobar1", "mock2-foobar2"),
			},
		},
		"existing group that previously had targets with no targets": {
			config: config,
			discoverers: []model.Discoverer{
				newMockDiscoverer("rule1",
					newMockTargetGroup("test", "mock1", "mock2"),
				),
				newDelayedMockDiscoverer("rule1", 5,
					newMockTargetGroup("test"),
				),
			},
			wantClassifyCalls: 2,
			wantComposeCalls:  2,
			wantConfGroups: []*confgroup.Group{
				prepareDiscoveredGroup("mock1-foobar1", "mock2-foobar2"),
				prepareDiscoveredGroup(),
			},
		},
		"existing group with old and new targets": {
			config: config,
			discoverers: []model.Discoverer{
				newMockDiscoverer("rule1",
					newMockTargetGroup("test", "mock1", "mock2"),
				),
				newDelayedMockDiscoverer("rule1", 5,
					newMockTargetGroup("test", "mock1", "mock2", "mock11", "mock22"),
				),
			},
			wantClassifyCalls: 4,
			wantComposeCalls:  4,
			wantConfGroups: []*confgroup.Group{
				prepareDiscoveredGroup("mock1-foobar1", "mock2-foobar2"),
				prepareDiscoveredGroup("mock1-foobar1", "mock2-foobar2", "mock11-foobar1", "mock22-foobar2"),
			},
		},
		"existing group with new targets only": {
			config: config,
			discoverers: []model.Discoverer{
				newMockDiscoverer("rule1",
					newMockTargetGroup("test", "mock1", "mock2"),
				),
				newDelayedMockDiscoverer("rule1", 5,
					newMockTargetGroup("test", "mock11", "mock22"),
				),
			},
			wantClassifyCalls: 4,
			wantComposeCalls:  4,
			wantConfGroups: []*confgroup.Group{
				prepareDiscoveredGroup("mock1-foobar1", "mock2-foobar2"),
				prepareDiscoveredGroup("mock11-foobar1", "mock22-foobar2"),
			},
		},
	}

	for name, sim := range tests {
		t.Run(name, func(t *testing.T) {
			sim.run(t)
		})
	}
}

func prepareDiscoveredGroup(configNames ...string) *confgroup.Group {
	var configs []confgroup.Config

	for _, name := range configNames {
		configs = append(configs, confgroup.Config{}.
			SetProvider("mock").
			SetSourceType(confgroup.TypeDiscovered).
			SetSource("test").
			SetName(name))
	}

	return &confgroup.Group{
		Source:  "test",
		Configs: configs,
	}
}

func newMockDiscoverer(tags string, tggs ...model.TargetGroup) *mockDiscoverer {
	return &mockDiscoverer{
		tags: mustParseTags(tags),
		tggs: tggs,
	}
}

func newDelayedMockDiscoverer(tags string, delay int, tggs ...model.TargetGroup) *mockDiscoverer {
	return &mockDiscoverer{
		tags:  mustParseTags(tags),
		tggs:  tggs,
		delay: time.Duration(delay) * time.Second,
	}
}

type mockDiscoverer struct {
	tggs  []model.TargetGroup
	tags  model.Tags
	delay time.Duration
}

func (md mockDiscoverer) String() string {
	return "mock discoverer"
}

func (md mockDiscoverer) Discover(ctx context.Context, out chan<- []model.TargetGroup) {
	for _, tgg := range md.tggs {
		for _, tgt := range tgg.Targets() {
			tgt.Tags().Merge(md.tags)
		}
	}

	select {
	case <-ctx.Done():
	case <-time.After(md.delay):
		select {
		case <-ctx.Done():
		case out <- md.tggs:
		}
	}
}

func newMockTargetGroup(source string, targets ...string) *mockTargetGroup {
	m := &mockTargetGroup{source: source}
	for _, name := range targets {
		m.targets = append(m.targets, &mockTarget{Name: name})
	}
	return m
}

type mockTargetGroup struct {
	targets []model.Target
	source  string
}

func (mg mockTargetGroup) Targets() []model.Target { return mg.targets }
func (mg mockTargetGroup) Source() string          { return mg.source }
func (mg mockTargetGroup) Provider() string        { return "mock" }

func newMockTarget(name string, tags ...string) *mockTarget {
	m := &mockTarget{Name: name}
	v, _ := model.ParseTags(strings.Join(tags, " "))
	m.Tags().Merge(v)
	return m
}

type mockTarget struct {
	model.Base
	Name string
}

func (mt mockTarget) TUID() string { return mt.Name }
func (mt mockTarget) Hash() uint64 { return mustCalcHash(mt.Name) }

func mustParseTags(line string) model.Tags {
	v, err := model.ParseTags(line)
	if err != nil {
		panic(fmt.Sprintf("mustParseTags: %v", err))
	}
	return v
}

func mustCalcHash(obj any) uint64 {
	hash, err := hashstructure.Hash(obj, nil)
	if err != nil {
		panic(fmt.Sprintf("hash calculation: %v", err))
	}
	return hash
}
