// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/confgroup"
	"github.com/netdata/netdata/go/go.d.plugin/agent/discovery/sd/model"

	"github.com/ilyam8/hashstructure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

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
				{Source: "test", Configs: []confgroup.Config{
					{
						"__provider__": "mock",
						"__source__":   "test",
						"name":         "mock1-foobar1",
					},
					{
						"__provider__": "mock",
						"__source__":   "test",
						"name":         "mock2-foobar2",
					},
				}},
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
				{Source: "test", Configs: []confgroup.Config{
					{
						"__provider__": "mock",
						"__source__":   "test",
						"name":         "mock1-foobar1",
					},
					{
						"__provider__": "mock",
						"__source__":   "test",
						"name":         "mock2-foobar2",
					},
				}},
			},
		},
		"existing empty group that previously had targets": {
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
				{Source: "test", Configs: []confgroup.Config{
					{
						"__provider__": "mock",
						"__source__":   "test",
						"name":         "mock1-foobar1",
					},
					{
						"__provider__": "mock",
						"__source__":   "test",
						"name":         "mock2-foobar2",
					},
				}},
				{Source: "test", Configs: nil},
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
				{Source: "test", Configs: []confgroup.Config{
					{
						"__provider__": "mock",
						"__source__":   "test",
						"name":         "mock1-foobar1",
					},
					{
						"__provider__": "mock",
						"__source__":   "test",
						"name":         "mock2-foobar2",
					},
				}},
				{Source: "test", Configs: []confgroup.Config{
					{
						"__provider__": "mock",
						"__source__":   "test",
						"name":         "mock1-foobar1",
					},
					{
						"__provider__": "mock",
						"__source__":   "test",
						"name":         "mock2-foobar2",
					},
					{
						"__provider__": "mock",
						"__source__":   "test",
						"name":         "mock11-foobar1",
					},
					{
						"__provider__": "mock",
						"__source__":   "test",
						"name":         "mock22-foobar2",
					},
				}},
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
				{Source: "test", Configs: []confgroup.Config{
					{
						"__provider__": "mock",
						"__source__":   "test",
						"name":         "mock1-foobar1",
					},
					{
						"__provider__": "mock",
						"__source__":   "test",
						"name":         "mock2-foobar2",
					},
				}},
				{Source: "test", Configs: []confgroup.Config{
					{
						"__provider__": "mock",
						"__source__":   "test",
						"name":         "mock11-foobar1",
					},
					{
						"__provider__": "mock",
						"__source__":   "test",
						"name":         "mock22-foobar2",
					},
				}},
			},
		},
	}

	for name, sim := range tests {
		t.Run(name, func(t *testing.T) {
			sim.run(t)
		})
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
