// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	secretconfig "github.com/netdata/netdata/go/plugins/plugin/agent/secrets"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
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
	for _, tc := range []struct {
		name, field string
		want        bool
	}{
		{"omitted", "", true},
		{"true", "allow_privileged: true\n", true},
		{"false", "allow_privileged: false\n", false},
		// Existing scalar YAML decoding maps explicit null to false.
		// The public schema accepts only booleans; this is not a new null option.
		{"null", "allow_privileged: null\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
