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

	"github.com/gohugoio/hashstructure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
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

	const servicesConfig = `
services:
  - id: "svc-foobar1"
    match: '{{ glob .Name "mock*1*" }}'
    config_template: |
      name: {{ .Name }}-foobar1
  - id: "svc-foobar2"
    match: '{{ glob .Name "mock*2*" }}'
    config_template: |
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
		"services-only: new group with targets": {
			config: servicesConfig,
			discoverers: []model.Discoverer{
				newMockDiscoverer("rule1",
					newMockTargetGroup("test", "mock1", "mock2"),
				),
			},
			useServices:       true, // tell the simulator to wire svr-only
			wantClassifyCalls: 0,    // no classify in services mode
			wantComposeCalls:  2,    // compose called per target (2 targets)
			wantConfGroups: []*confgroup.Group{
				// same expected configs as the legacy "new group with targets"
				prepareDiscoveredGroupWithModule("mock1-foobar1", "svc-foobar1", "mock2-foobar2", "svc-foobar2"),
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

func prepareDiscoveredGroupWithModule(values ...string) *confgroup.Group {
	var configs []confgroup.Config

	for i := 0; i < len(values); i += 2 {
		cfgName := values[i]
		modName := values[i+1]
		configs = append(configs, confgroup.Config{}.
			SetProvider("mock").
			SetSourceType(confgroup.TypeDiscovered).
			SetSource("test").
			SetName(cfgName).
			SetModule(modName),
		)
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

func TestConvertOldToServices(t *testing.T) {
	type inYAML struct {
		Classify string
		Compose  string
	}

	tests := map[string]struct {
		in   inYAML
		want []ServiceRuleConfig
	}{
		"basic 1:1 mapping": {
			in: inYAML{
				Classify: `
- name: "Applications"
  selector: "unknown"
  tags: "-unknown app"
  match:
    - tags: "activemq"
      expr: '{{ and (eq .Port "8161") (eq .Comm "activemq") }}'
`,
				Compose: `
- name: "Applications"
  selector: "app"
  config:
    - selector: "activemq"
      template: |
        module: activemq
        name: local
        url: http://{{.Address}}
        webadmin: admin
`,
			},
			want: []ServiceRuleConfig{
				{
					ID:             "activemq",
					Match:          `{{ and (eq .Port "8161") (eq .Comm "activemq") }}`,
					ConfigTemplate: "module: activemq\nname: local\nurl: http://{{.Address}}\nwebadmin: admin\n",
				},
			},
		},

		"multiple classify exprs for same tag -> multiple service rules": {
			in: inYAML{
				Classify: `
- name: "Databases"
  selector: "unknown"
  match:
    - tags: "redis"
      expr: '{{ eq .Port "6379" }}'
    - tags: "redis"
      expr: '{{ and (eq .Comm "redis-server") (eq .Address "127.0.0.1") }}'
`,
				Compose: `
- name: "Databases"
  selector: "app"
  config:
    - selector: "redis"
      template: |
        module: redis
        name: {{ .Name }}
`,
			},
			// NOTE: Order should follow classify expr encounter order:
			// 1) Port-based, 2) Comm+Address-based. IDs redis, redis_2 accordingly.
			want: []ServiceRuleConfig{
				{
					ID:             "redis",
					Match:          `{{ eq .Port "6379" }}`,
					ConfigTemplate: "module: redis\nname: {{ .Name }}\n",
				},
				{
					ID:             "redis_2",
					Match:          `{{ and (eq .Comm "redis-server") (eq .Address "127.0.0.1") }}`,
					ConfigTemplate: "module: redis\nname: {{ .Name }}\n",
				},
			},
		},

		"ignore deletions and rule-level tags without expr": {
			in: inYAML{
				Classify: `
- name: "NoExpr"
  selector: "unknown"
  tags: "nginx"     # rule-level tag: no expr -> cannot produce a service rule
  match: []
- name: "Deletions"
  selector: "unknown"
  match:
    - tags: "-nginx"               # deletion: ignore
      expr: '{{ eq .Port "80" }}'  # has expr but tag is a deletion, ignore
`,
				Compose: `
- name: "Web"
  selector: "app"
  config:
    - selector: "nginx"
      template: |
        module: nginx
        name: web
`,
			},
			want: nil, // nothing to map
		},

		"compose selector without classify producer -> empty": {
			in: inYAML{
				Classify: `
- name: "App"
  selector: "unknown"
  match:
    - tags: "foo"
      expr: '{{ eq .Port "1234" }}'
`,
				Compose: `
- name: "App"
  selector: "app"
  config:
    - selector: "bar"
      template: |
        module: bar
`,
			},
			want: nil,
		},

		"multiple compose selectors map to different classify tags": {
			in: inYAML{
				Classify: `
- name: "Mixed"
  selector: "unknown"
  match:
    - tags: "kafka"
      expr: '{{ eq .Port "9092" }}'
    - tags: "zookeeper"
      expr: '{{ eq .Port "2181" }}'
`,
				Compose: `
- name: "Stream"
  selector: "app"
  config:
    - selector: "zookeeper"
      template: |
        module: zookeeper
        name: zk
    - selector: "kafka"
      template: |
        module: kafka
        name: broker
`,
			},
			// Order follows compose config order; for each selector, classify exprs order is preserved.
			want: []ServiceRuleConfig{
				{
					ID:             "zookeeper",
					Match:          `{{ eq .Port "2181" }}`,
					ConfigTemplate: "module: zookeeper\nname: zk\n",
				},
				{
					ID:             "kafka",
					Match:          `{{ eq .Port "9092" }}`,
					ConfigTemplate: "module: kafka\nname: broker\n",
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var cls []ClassifyRuleConfig
			var cmp []ComposeRuleConfig

			require.NoError(t, yaml.Unmarshal([]byte(tc.in.Classify), &cls), "classify YAML")
			require.NoError(t, yaml.Unmarshal([]byte(tc.in.Compose), &cmp), "compose YAML")

			got, err := ConvertOldToServices(cls, cmp)
			require.NoError(t, err)

			assert.Equal(t, tc.want, got)
		})
	}
}
