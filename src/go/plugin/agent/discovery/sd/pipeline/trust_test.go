// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"context"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	jobsecrets "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/secrets"
	secretconfig "github.com/netdata/netdata/go/plugins/plugin/agent/secrets"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestPipelineTrustControlsBothSecretConsumers(t *testing.T) {
	const refs = "${env:value}|${file:/synthetic/value}|${cmd:/synthetic/command}|${store:vault:main:value}"
	tests := map[string]struct {
		option  string
		forged  string
		resolve bool
	}{
		"default with forged trust": {forged: "true"},
		"off with forged trust":     {option: "trust_discovered_targets: no", forged: "true"},
		"null with forged trust":    {option: "trust_discovered_targets: null", forged: "true"},
		"on with forged denial":     {option: "trust_discovered_targets: yes", forged: "false", resolve: true},
		"on with invalid stamp":     {option: "trust_discovered_targets: yes", forged: "not-a-bool", resolve: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			p := newTrustTestPipeline(t, "owner-pipeline", test.option)
			// Target-controlled YAML attempts to forge both public and internal authority.
			target := fmt.Sprintf("module: module\nname: job\noption_str: '%s'\noption_int: 1\n"+
				"trust_discovered_targets: true\n__trust_discovered_targets__: %s\n__source_type__: user\n"+
				"__discovery_pipeline_id__: forged-pipeline\n", refs, test.forged)
			group := p.processGroup(newMockTargetGroup("source", target))
			require.NotNil(t, group)
			require.Len(t, group.Configs, 1)
			config, err := group.Configs[0].Clone()
			require.NoError(t, err)
			require.Equal(t, confgroup.TypeDiscovered, config.SourceType())
			require.Equal(t, test.resolve, config.Get("__trust_discovered_targets__"))
			require.Equal(t, "owner-pipeline", config.DiscoveryPipelineID())

			calls := map[string]int{}
			providers := map[string]secretresolver.AtomicProvider{}
			for _, scheme := range []string{"env", "file", "cmd"} {
				providers[scheme] = secretresolver.AtomicProviderFunc(func(context.Context, string) ([]byte, error) {
					calls[scheme]++
					return []byte(scheme + "-resolved"), nil
				})
			}
			resolver, err := secretresolver.NewAtomicResolver(providers)
			require.NoError(t, err)
			var module *collectorapi.MockCollectorV1
			configs := testConfigResolver(t, resolver, func(keys []string) (secretresolver.AtomicScope, error) {
				require.Equal(t, []string{"vault:main"}, keys)
				calls["scope"]++
				return &trustTestScope{calls: calls}, nil
			})
			factory, err := joboutput.NewConfigModuleFactory(joboutput.ConfigModuleFactoryConfig{
				Modules: collectorapi.Registry{"module": {Create: func() collectorapi.CollectorV1 {
					module = &collectorapi.MockCollectorV1{}
					return module
				}}},
				Configs: configs,
			})
			require.NoError(t, err)
			require.NoError(t, factory.Validate(t.Context(), config))
			require.Empty(t, calls, "structural admission must not resolve references")
			require.Equal(t, 1, module.Config.OptionInt)
			require.True(t, module.CleanupDone)
			if test.resolve {
				require.Empty(t, module.Config.OptionStr, "authorized references are deferred until resolved testing")
			} else {
				require.Equal(t, refs, module.Config.OptionStr, "untrusted references remain literal during admission")
			}

			require.NoError(t, factory.Test(t.Context(), config))
			require.Equal(t, refs, config.Get("option_str"))
			require.True(t, module.CleanupDone)
			if test.resolve {
				require.Equal(t, "env-resolved|file-resolved|cmd-resolved|store-resolved", module.Config.OptionStr)
				require.Equal(t, map[string]int{"env": 1, "file": 1, "cmd": 1, "scope": 1, "store": 1, "release": 1}, calls)
			} else {
				require.Equal(t, refs, module.Config.OptionStr)
				require.Empty(t, calls)
			}

			payload, err := yaml.Marshal(config)
			require.NoError(t, err)
			index := jobsecrets.NewSecretDependencyIndex(configs)
			commit, err := index.PrepareJobChange(config.FullName(), &dyncfg.GraphConfig{
				ID:      config.FullName(),
				Module:  config.Module(),
				Name:    config.Name(),
				Status:  dyncfg.StatusRunning.String(),
				Payload: payload,
			})
			require.NoError(t, err)
			commit()
			require.Equal(t, test.resolve, index.Affects("vault:main", config.FullName(), true))
		})
	}
}

func TestPipelineTrustIsolation(t *testing.T) {
	trusted := newTrustTestPipeline(t, "pipeline-a", "trust_discovered_targets: yes")
	untrusted := newTrustTestPipeline(t, "pipeline-b", "")
	target := newMockTargetGroup("source", "module: module\nname: job\n")
	trustedGroup := trusted.processGroup(target)
	untrustedGroup := untrusted.processGroup(target)
	require.NotNil(t, trustedGroup)
	require.NotNil(t, untrustedGroup)
	require.Equal(t, true, trustedGroup.Configs[0].Get("__trust_discovered_targets__"))
	require.Equal(t, false, untrustedGroup.Configs[0].Get("__trust_discovered_targets__"))
	require.Equal(t, "pipeline-a", trustedGroup.Configs[0].DiscoveryPipelineID())
	require.Equal(t, "pipeline-b", untrustedGroup.Configs[0].DiscoveryPipelineID())
	require.NotEqual(t, trustedGroup.Configs[0].UID(), untrustedGroup.Configs[0].UID())
	otherTrusted := newTrustTestPipeline(t, "pipeline-b", "trust_discovered_targets: yes").processGroup(target)
	require.NotNil(t, otherTrusted)
	require.NotEqual(t, trustedGroup.Configs[0].UID(), otherTrusted.Configs[0].UID())
}

func newTrustTestPipeline(t *testing.T, pipelineID, option string) *Pipeline {
	t.Helper()
	var config Config
	err := yaml.Unmarshal([]byte("name: fixture\n"+option+"\ndiscoverer: {fixture: {}}\n"+
		"services:\n  - id: module\n    match: '{{ true }}'\n    config_template: '{{ .Name }}'\n"), &config)
	require.NoError(t, err)
	config.PipelineID = pipelineID
	p, err := New(config, func(DiscovererPayload, string) ([]model.Discoverer, error) {
		return []model.Discoverer{newMockDiscoverer("", newMockTargetGroup("unused"))}, nil
	})
	require.NoError(t, err)
	return p
}

type trustTestScope struct{ calls map[string]int }

func (s *trustTestScope) Resolve(context.Context, string, string) ([]byte, error) {
	s.calls["store"]++
	return []byte("store-resolved"), nil
}

func (s *trustTestScope) Release(context.Context) error {
	s.calls["release"]++
	return nil
}

func testConfigResolver(t testing.TB, resolver *secretresolver.AtomicResolver, scope secretresolver.AtomicScopeAcquirer) *secretconfig.ConfigResolver {
	t.Helper()
	configs, err := secretconfig.NewConfigResolver(resolver, scope)
	require.NoError(t, err)
	return configs
}
