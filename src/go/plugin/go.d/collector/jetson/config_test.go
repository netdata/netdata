// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretconfig "github.com/netdata/netdata/go/plugins/plugin/agent/secrets"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/streamexec/streamexectest"
)

// Use the production factory's constructor, decode, and pre-Init configuration
// retrieval. Only executable discovery is replaced; no initialized policy is injected.
func configFactory(t *testing.T, fake *streamexectest.Fake) (*joboutput.ConfigModuleFactory, **Collector) {
	t.Helper()
	var current *Collector
	creator := collectorapi.DefaultRegistry["jetson"]
	create := creator.CreateV2
	creator.CreateV2 = func() collectorapi.CollectorV2 {
		current = create().(*Collector)
		current.findTegrastats = func() (string, error) { return fake.Binary, nil }
		current.fixedTegrastatsExists = func() bool { t.Fatal("PATH was available; fixed-path discovery was unnecessary"); return false }
		return current
	}
	factory, err := joboutput.NewConfigModuleFactory(joboutput.ConfigModuleFactoryConfig{
		Modules: collectorapi.Registry{"jetson": creator}, Configs: &secretconfig.ConfigResolver{},
	})
	require.NoError(t, err)
	return factory, &current
}

func configuredCollector(t *testing.T, fake *streamexectest.Fake, config confgroup.Config) *Collector {
	t.Helper()
	config.Set("module", "jetson")
	config.ApplyDefaults(confgroup.Default{})
	factory, current := configFactory(t, fake)
	require.NoError(t, factory.Test(t.Context(), config))
	return *current
}

func TestConfigurationFactoryDefaultsAndReload(t *testing.T) {
	for name, tc := range map[string]struct {
		field string
		want  bool
	}{
		"omitted": {"", true},
		"true":    {"allow_privileged: true\n", true},
		"false":   {"allow_privileged: false\n", false},
		// Existing scalar YAML decoding maps explicit null to false.
		// The public schema accepts only booleans; this is not a new null option.
		"null": {"allow_privileged: null\n", false},
	} {
		t.Run(name, func(t *testing.T) {
			peer, fake := newSupervisedPeer(t, "success")
			factory, current := configFactory(t, fake)
			var cfg confgroup.Config
			require.NoError(t, yaml.Unmarshal([]byte("module: jetson\nname: factory.job\n"+tc.field), &cfg))
			cfg.ApplyDefaults(confgroup.Default{})
			payload, err := factory.Configuration(t.Context(), cfg)
			require.NoError(t, err)
			assert.Nil(t, (*current).acquisition, "configuration retrieval must not initialize acquisition")
			var reloaded confgroup.Config
			require.NoError(t, json.Unmarshal(payload, &reloaded))
			require.Contains(t, reloaded, "allow_privileged", "explicit false must be serialized")
			assert.Equal(t, tc.want, reloaded["allow_privileged"])
			require.Equal(t, "factory_job", reloaded["name"])
			reloaded.Set("module", "jetson")
			reloaded.ApplyDefaults(confgroup.Default{})
			require.NoError(t, factory.Test(t.Context(), reloaded))
			assert.Equal(t, 0, peer.attempts(), "Init and Check must not execute a helper")
			fake.RequireNoStart(t, 10*time.Millisecond)
			stop := startRun(t, *current)
			fake.Accept(t)
			stop()
			wantAttempts := 0
			if tc.want {
				wantAttempts = 1
			}
			assert.Equal(t, wantAttempts, peer.attempts())
		})
	}
}

func TestCapturedOwnerAndDisabledPolicy(t *testing.T) {
	peer, fake := newSupervisedPeer(t, "success")
	c := configuredCollector(t, fake, confgroup.Config{"name": "owned.job", "allow_privileged": false})
	// Initialization captures both values once, before any producer starts.
	c.Name, c.AllowPrivileged = "changed", true
	stop := startRun(t, c)
	fake.Accept(t)
	same := configuredCollector(t, fake, confgroup.Config{"name": "owned:job", "allow_privileged": false})
	requireStartError(t, same, ndexec.ErrAcquisitionBusy, "")
	renamed := configuredCollector(t, fake, confgroup.Config{"name": "changed", "allow_privileged": false})
	stopRenamed := startRun(t, renamed)
	fake.Accept(t)
	stopRenamed()
	stop()
	assert.Zero(t, peer.attempts(), "captured false must not invoke the helper")
}

func TestAnonymousAndNamedOwnersAreDistinct(t *testing.T) {
	peer, fake := newSupervisedPeer(t, "success")
	anonymous := policyCollector(t, fake)
	anonymous.Name, anonymous.AllowPrivileged = "", false
	stop := startRun(t, anonymous)
	fake.Accept(t)
	duplicate := policyCollector(t, fake)
	duplicate.Name, duplicate.AllowPrivileged = "", false
	requireStartError(t, duplicate, ndexec.ErrAcquisitionBusy, "")
	named := configuredCollector(t, fake, confgroup.Config{"name": "anonymous", "allow_privileged": false})
	stopNamed := startRun(t, named)
	fake.Accept(t)
	stopNamed()
	stop()
	assert.Zero(t, peer.attempts())
}

func TestUserConfigExportAndReload(t *testing.T) {
	for _, format := range []string{"application/json", "application/yaml"} {
		for name, tc := range map[string]struct {
			json, yaml string
			want       bool
		}{
			"omitted": {`{"update_every":1}`, "update_every: 1\n", true},
			"true":    {`{"allow_privileged":true}`, "allow_privileged: true\n", true},
			"false":   {`{"allow_privileged":false}`, "allow_privileged: false\n", false},
		} {
			t.Run(format+"/"+name, func(t *testing.T) {
				peer, fake := newSupervisedPeer(t, "success")
				factory, _ := configFactory(t, fake)
				graph, err := dyncfg.NewGraph(nil)
				require.NoError(t, err)
				output := &bytes.Buffer{}
				frames, err := lifecycle.NewFrameOwner(output)
				require.NoError(t, err)
				controller, err := joboutput.NewDynCfgJobController(joboutput.DynCfgJobControllerConfig{
					PluginName: "go.d", Modules: collectorapi.Registry{"jetson": collectorapi.DefaultRegistry["jetson"]},
					Defaults: confgroup.Registry{}, ConfigModules: factory, Graph: graph, Frames: frames,
					// USERCONFIG only exports the registered config. Job activation is unused.
					Factory: &joboutput.Factory{},
				})
				require.NoError(t, err)
				payload := tc.json
				if format == "application/yaml" {
					payload = tc.yaml
				}
				result, err := controller.Handle(t.Context(), joboutput.DynCfgJobRequest{
					Args:        []string{"go.d:collector:jetson", string(dyncfg.CommandUserconfig), "exported"},
					ContentType: format, HasPayload: true, Payload: []byte(payload),
				})
				require.NoError(t, err)
				frame, err := lifecycle.PrepareFrame("export", result, 1)
				require.NoError(t, err)
				require.NoError(t, frames.Commit(frame))
				header, rest, ok := strings.Cut(output.String(), "\n")
				require.True(t, ok)
				require.Contains(t, header, "200 application/yaml")
				body, _, ok := strings.Cut(rest, "\nFUNCTION_RESULT_END")
				require.True(t, ok)
				var exported struct {
					Jobs []confgroup.Config `yaml:"jobs"`
				}
				require.NoError(t, yaml.Unmarshal([]byte(body), &exported))
				require.Len(t, exported.Jobs, 1)
				require.Contains(t, exported.Jobs[0], "allow_privileged")
				require.Equal(t, tc.want, exported.Jobs[0]["allow_privileged"])
				c := configuredCollector(t, fake, exported.Jobs[0])
				require.Equal(t, "exported", c.Name)
				stop := startRun(t, c)
				fake.Accept(t)
				stop()
				wantAttempts := 0
				if tc.want {
					wantAttempts = 1
				}
				assert.Equal(t, wantAttempts, peer.attempts())
			})
		}
	}
}
