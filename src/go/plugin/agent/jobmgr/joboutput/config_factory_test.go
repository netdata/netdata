// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretconfig "github.com/netdata/netdata/go/plugins/plugin/agent/secrets"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestConfigModuleFactoryCleansEveryAttemptAndPrefersV2(t *testing.T) {
	tests := map[string]struct {
		operation   string
		checkErr    error
		wantErr     bool
		wantCreates int
	}{
		"configuration success": {operation: "configuration", wantCreates: 1},
		"test success":          {operation: "test", wantCreates: 1},
		"test failure":          {operation: "test", checkErr: errors.New("check failed"), wantErr: true, wantCreates: 1},
		"validation success":    {operation: "validate", wantCreates: 1},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			state := &factoryTestState{}
			var module *factoryTestV2
			v1Creates := 0
			v2Creates := 0
			resolver, err := secretresolver.NewAtomicResolver(nil)
			require.NoError(t, err)
			factory, err := NewConfigModuleFactory(
				ConfigModuleFactoryConfig{
					Modules: collectorapi.Registry{
						"module": {
							Create: func() collectorapi.CollectorV1 {
								v1Creates++
								return state.module(nil, false)
							},
							CreateV2: func() collectorapi.CollectorV2 {
								v2Creates++
								module = &factoryTestV2{
									state:    state,
									checkErr: test.checkErr,
								}
								return module
							},
						},
					},
					Configs: testConfigResolver(t, resolver, unavailableStoreScope),
				},
			)
			require.NoError(t, err)
			config := factoryTestConfig(false)
			switch test.operation {
			case "configuration":
				payload, runErr := factory.Configuration(context.Background(), config)
				err = runErr
				require.False(t, runErr == nil && !json.Valid(payload))
			case "test":
				err = factory.Test(context.Background(), config)
			case "validate":
				err = factory.Validate(context.Background(), config)
			default:
				require.FailNowf(t, "test failed", "unknown operation %q", test.operation)
			}
			require.EqualValues(t, test.wantErr, err != nil)
			require.Equal(t, config.Name(), module.Name)
			require.False(
				t,
				v1Creates != 0 || v2Creates != test.wantCreates || state.collectorCleanup != test.wantCreates,
			)
		})
	}
}

func TestConfigModuleFactoryRedactsResolvedValuesFromDecodeErrors(t *testing.T) {
	const resolvedFixture = "resolved-sensitive-fixture"
	resolver, err := secretresolver.NewAtomicResolver(map[string]secretresolver.AtomicProvider{
		"fixture": secretresolver.AtomicProviderFunc(
			func(context.Context, string) ([]byte, error) {
				return []byte(resolvedFixture), nil
			},
		),
	})
	require.NoError(t, err)
	factory, err := NewConfigModuleFactory(ConfigModuleFactoryConfig{
		Modules: collectorapi.Registry{
			"module": {
				Create: func() collectorapi.CollectorV1 {
					return &collectorapi.MockCollectorV1{}
				},
			},
		},
		Configs: testConfigResolver(t, resolver, unavailableStoreScope),
	})
	require.NoError(t, err)
	config := factoryTestConfig(false)
	config["option_int"] = "${fixture:value}"

	err = factory.Test(context.Background(), config)
	require.Error(t, err)
	require.NotContains(t, err.Error(), resolvedFixture)
	require.True(t, strings.Contains(err.Error(), "resolved") && strings.Contains(err.Error(), "redacted"))
}

func TestConfigModuleFactorySecretReferenceSourcePolicy(t *testing.T) {
	const references = "${env:value}|${file:/synthetic/value}|${cmd:/synthetic/command}|${store:vault:main:value}"
	tests := map[string]struct {
		sourceType string
		trust      bool
		value      string
		resolve    bool
	}{
		"stock":                {sourceType: confgroup.TypeStock, value: references, resolve: true},
		"user":                 {sourceType: confgroup.TypeUser, value: references, resolve: true},
		"dyncfg":               {sourceType: confgroup.TypeDyncfg, value: references, resolve: true},
		"trusted discovered":   {sourceType: confgroup.TypeDiscovered, trust: true, value: references, resolve: true},
		"discovered":           {sourceType: confgroup.TypeDiscovered, value: references},
		"discovered malformed": {sourceType: confgroup.TypeDiscovered, value: "${store:invalid}|${env:}"},
		"empty":                {value: references},
		"unknown":              {sourceType: "future-source", value: references},
	}
	variants := map[string]func() (collectorapi.Creator, func() string){
		"V1": func() (collectorapi.Creator, func() string) {
			var module *collectorapi.MockCollectorV1
			return collectorapi.Creator{
				Create: func() collectorapi.CollectorV1 {
					module = &collectorapi.MockCollectorV1{}
					return module
				},
			}, func() string { return module.Config.OptionStr }
		},
		"V2": func() (collectorapi.Creator, func() string) {
			var module *factoryTestV2
			return collectorapi.Creator{
				CreateV2: func() collectorapi.CollectorV2 {
					module = &factoryTestV2{state: &factoryTestState{}}
					return module
				},
			}, func() string { return module.OptionStr }
		},
	}
	for name, test := range tests {
		for variantName, newVariant := range variants {
			for _, operation := range []string{"validate", "test", "snapshot"} {
				t.Run(name+"/"+variantName+"/"+operation, func(t *testing.T) {
					providerCalls := map[string]int{}
					scopeCalls := 0
					providers := map[string]secretresolver.AtomicProvider{}
					for _, scheme := range []string{"env", "file", "cmd"} {
						providers[scheme] = secretresolver.AtomicProviderFunc(func(context.Context, string) ([]byte, error) {
							providerCalls[scheme]++
							return []byte(scheme + "-resolved"), nil
						})
					}
					resolver, err := secretresolver.NewAtomicResolver(providers)
					require.NoError(t, err)
					creator, option := newVariant()
					factory, err := NewConfigModuleFactory(ConfigModuleFactoryConfig{
						Modules: collectorapi.Registry{"module": creator},
						Configs: testConfigResolver(t, resolver, func([]string) (secretresolver.AtomicScope, error) {
							scopeCalls++
							return &factoryTestAtomicScope{value: "store-resolved"}, nil
						}),
					})
					require.NoError(t, err)
					config := factoryTestConfig(false)
					config.SetSourceType(test.sourceType).SetTrustDiscoveredTargets(test.trust)
					config["option_str"] = test.value
					config["option_int"] = 1

					if operation == "validate" {
						err = factory.Validate(context.Background(), config)
					} else if operation == "test" {
						err = factory.Test(context.Background(), config)
					} else {
						probe, constructErr := factory.construct(config.Module())
						require.NoError(t, constructErr)
						defer probe.cleanup(context.Background())
						var snapshot secretresolver.AtomicScopeSnapshot
						var redact bool
						redact, snapshot, err = factory.applyResolvedWithSnapshot(context.Background(), config, probe.module)
						require.Equal(t, test.resolve, redact)
						require.Equal(t, test.resolve, snapshot != nil)
					}
					require.NoError(t, err)
					require.Equal(t, test.value, config.Get("option_str"))
					if operation == "validate" && test.resolve {
						require.Empty(t, option())
						require.Empty(t, providerCalls)
						require.Zero(t, scopeCalls)
					} else if test.resolve {
						require.Equal(t, "env-resolved|file-resolved|cmd-resolved|store-resolved", option())
						require.Equal(t, map[string]int{"env": 1, "file": 1, "cmd": 1}, providerCalls)
						require.Equal(t, 1, scopeCalls)
					} else {
						require.Equal(t, test.value, option())
						require.Empty(t, providerCalls)
						require.Zero(t, scopeCalls)
					}
				})
			}
		}
	}
}

func TestConfigModuleFactoryRedactsReferenceResolutionFailures(t *testing.T) {
	const (
		resolverSensitive = "resolver-sensitive-fixture"
		cleanupSensitive  = "cleanup-sensitive-fixture"
	)
	tests := map[string]struct {
		resolver   func(*testing.T) *secretresolver.AtomicResolver
		storeScope secretresolver.AtomicScopeAcquirer
		reference  string
	}{
		"provider error": {
			resolver: func(t *testing.T) *secretresolver.AtomicResolver {
				resolver, err := secretresolver.NewAtomicResolver(map[string]secretresolver.AtomicProvider{
					"fixture": secretresolver.AtomicProviderFunc(func(context.Context, string) ([]byte, error) {
						return nil, errors.New(resolverSensitive)
					}),
				})
				require.NoError(t, err)
				return resolver
			},
			storeScope: unavailableStoreScope,
			reference:  "${fixture:value}",
		},
		"provider panic": {
			resolver: func(t *testing.T) *secretresolver.AtomicResolver {
				resolver, err := secretresolver.NewAtomicResolver(map[string]secretresolver.AtomicProvider{
					"fixture": secretresolver.AtomicProviderFunc(func(context.Context, string) ([]byte, error) {
						panic(resolverSensitive)
					}),
				})
				require.NoError(t, err)
				return resolver
			},
			storeScope: unavailableStoreScope,
			reference:  "${fixture:value}",
		},
		"store acquire error": {
			resolver: func(t *testing.T) *secretresolver.AtomicResolver {
				resolver, err := secretresolver.NewAtomicResolver(nil)
				require.NoError(t, err)
				return resolver
			},
			storeScope: func([]string) (secretresolver.AtomicScope, error) {
				return nil, errors.New(resolverSensitive)
			},
			reference: "${store:vault:main:key}",
		},
		"store resolve error": {
			resolver: func(t *testing.T) *secretresolver.AtomicResolver {
				resolver, err := secretresolver.NewAtomicResolver(nil)
				require.NoError(t, err)
				return resolver
			},
			storeScope: func([]string) (secretresolver.AtomicScope, error) {
				return &sensitiveConfigFactoryScope{resolveErr: errors.New(resolverSensitive)}, nil
			},
			reference: "${store:vault:main:key}",
		},
		"store release error": {
			resolver: func(t *testing.T) *secretresolver.AtomicResolver {
				resolver, err := secretresolver.NewAtomicResolver(nil)
				require.NoError(t, err)
				return resolver
			},
			storeScope: func([]string) (secretresolver.AtomicScope, error) {
				return &sensitiveConfigFactoryScope{releaseErr: errors.New(resolverSensitive)}, nil
			},
			reference: "${store:vault:main:key}",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			factory, err := NewConfigModuleFactory(ConfigModuleFactoryConfig{
				Modules: collectorapi.Registry{
					"module": {
						Create: func() collectorapi.CollectorV1 {
							return &collectorapi.MockCollectorV1{
								CleanupFunc: func(context.Context) {
									panic(cleanupSensitive)
								},
							}
						},
					},
				},
				Configs: testConfigResolver(t, test.resolver(t), test.storeScope),
			})
			require.NoError(t, err)
			config := factoryTestConfig(false)
			config["option_str"] = test.reference
			config["option_int"] = 1

			err = factory.Test(context.Background(), config)
			require.Error(t, err)
			require.NotContains(t, err.Error(), resolverSensitive)
			require.NotContains(t, err.Error(), cleanupSensitive)
			require.Contains(t, err.Error(), "redacted")
		})
	}
}

func TestConfigModuleFactoryRedactsResolvedValuesFromCollectorLifecycle(t *testing.T) {
	const resolvedFixture = "resolved-sensitive-fixture"
	for _, phase := range []string{"init", "check", "cleanup"} {
		t.Run(phase, func(t *testing.T) {
			resolver, err := secretresolver.NewAtomicResolver(map[string]secretresolver.AtomicProvider{
				"fixture": secretresolver.AtomicProviderFunc(
					func(context.Context, string) ([]byte, error) {
						return []byte(resolvedFixture), nil
					},
				),
			})
			require.NoError(t, err)
			var module *collectorapi.MockCollectorV1
			factory, err := NewConfigModuleFactory(ConfigModuleFactoryConfig{
				Modules: collectorapi.Registry{
					"module": {
						Create: func() collectorapi.CollectorV1 {
							module = &collectorapi.MockCollectorV1{}
							if phase == "init" {
								module.InitFunc = func(context.Context) error {
									return fmt.Errorf("init exposed %s", module.Config.OptionStr)
								}
							}
							if phase == "check" {
								module.CheckFunc = func(context.Context) error {
									return fmt.Errorf("check exposed %s", module.Config.OptionStr)
								}
							}
							if phase == "cleanup" {
								module.CleanupFunc = func(context.Context) {
									panic("cleanup exposed " + module.Config.OptionStr)
								}
							}
							return module
						},
					},
				},
				Configs: testConfigResolver(t, resolver, unavailableStoreScope),
			})
			require.NoError(t, err)
			config := factoryTestConfig(false)
			config["option_str"] = "${fixture:value}"
			config["option_int"] = 1

			err = factory.Test(context.Background(), config)
			require.Error(t, err)
			require.NotContains(t, err.Error(), resolvedFixture)
			require.Contains(t, err.Error(), "redacted")
		})
	}
}

func TestConfigModuleFactoryRedactsCollectorInternalLogsAfterResolution(t *testing.T) {
	const resolvedFixture = "resolved-config-module-log-fixture"
	resolver, err := secretresolver.NewAtomicResolver(map[string]secretresolver.AtomicProvider{
		"fixture": secretresolver.AtomicProviderFunc(
			func(context.Context, string) ([]byte, error) {
				return []byte(resolvedFixture), nil
			},
		),
	})
	require.NoError(t, err)
	var module *collectorapi.MockCollectorV1
	factory, err := NewConfigModuleFactory(ConfigModuleFactoryConfig{
		Modules: collectorapi.Registry{
			"module": {
				Create: func() collectorapi.CollectorV1 {
					module = &collectorapi.MockCollectorV1{
						CheckFunc: func(context.Context) error {
							module.Warningf("collector log exposed %s", module.Config.OptionStr)
							return nil
						},
					}
					return module
				},
			},
		},
		Configs: testConfigResolver(t, resolver, unavailableStoreScope),
	})
	require.NoError(t, err)
	var logs bytes.Buffer
	factory.logger = logger.NewWithWriter(&logs)
	config := factoryTestConfig(false)
	config["option_str"] = "${fixture:value}"
	config["option_int"] = 1

	require.NoError(t, factory.Test(context.Background(), config))
	require.NotContains(t, logs.String(), resolvedFixture)
	require.Contains(t, logs.String(), "redacted")
}

func TestResolvedLifecycleRedactionPreservesControlClassifications(t *testing.T) {
	tests := map[string]struct {
		classify func(error) error
		want     collectorapi.LifecycleErrorClass
	}{
		"permanent": {classify: collectorapi.PermanentError, want: collectorapi.LifecycleErrorPermanent},
		"temporary": {classify: collectorapi.TemporaryError, want: collectorapi.LifecycleErrorTemporary},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := lifecycle.RetainOwnership(errors.Join(
				lifecycle.ErrTaskPanic,
				context.Canceled,
				test.classify(errors.New("resolved-sensitive-fixture")),
			))

			redacted := redactResolvedLifecycleError(err)

			require.NotContains(t, redacted.Error(), "resolved-sensitive-fixture")
			require.Contains(t, redacted.Error(), "redacted")
			require.ErrorIs(t, redacted, lifecycle.ErrTaskPanic)
			require.ErrorIs(t, redacted, context.Canceled)
			require.True(t, lifecycle.OwnershipRetained(redacted))
			require.Equal(t, test.want, collectorapi.ClassifyLifecycleError(redacted))
		})
	}
}

func TestResolvedLifecycleRedactionPreservesPureProcessControlTrees(t *testing.T) {
	tests := map[string]struct {
		err  error
		want []error
	}{
		"retired": {
			err:  fmt.Errorf("resolved-sensitive-retired: %w", jobmgr.ErrProcessAttemptRetired),
			want: []error{jobmgr.ErrProcessAttemptRetired},
		},
		"stopped": {
			err:  fmt.Errorf("resolved-sensitive-stopped: %w", jobmgr.ErrProcessAttemptStopped),
			want: []error{jobmgr.ErrProcessAttemptStopped},
		},
		"joined": {
			err: errors.Join(
				fmt.Errorf("resolved-sensitive-retired: %w", jobmgr.ErrProcessAttemptRetired),
				fmt.Errorf("resolved-sensitive-stopped: %w", jobmgr.ErrProcessAttemptStopped),
			),
			want: []error{jobmgr.ErrProcessAttemptRetired, jobmgr.ErrProcessAttemptStopped},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			redacted := redactResolvedLifecycleError(test.err)

			require.Contains(t, redacted.Error(), "redacted")
			require.NotContains(t, redacted.Error(), "resolved-sensitive")
			for _, want := range test.want {
				require.ErrorIs(t, redacted, want)
			}
			require.True(t, jobmgr.ContainsOnlyErrorLeaves(
				redacted,
				jobmgr.ErrProcessAttemptRetired,
				jobmgr.ErrProcessAttemptStopped,
			))
		})
	}
}

func TestResolvedLifecycleRedactionRejectsMixedProcessControlTree(t *testing.T) {
	err := errors.Join(
		fmt.Errorf("resolved-sensitive-stopped: %w", jobmgr.ErrProcessAttemptStopped),
		errors.New("resolved-sensitive-operational"),
	)

	redacted := redactResolvedLifecycleError(err)

	require.Contains(t, redacted.Error(), "redacted")
	require.NotContains(t, redacted.Error(), "resolved-sensitive")
	require.NotErrorIs(t, redacted, jobmgr.ErrProcessAttemptStopped)
	require.False(t, jobmgr.ContainsOnlyErrorLeaves(
		redacted,
		jobmgr.ErrProcessAttemptRetired,
		jobmgr.ErrProcessAttemptStopped,
	))
}

func TestResolvedLifecycleRedactionComposesProcessControlMetadata(t *testing.T) {
	err := lifecycle.RetainOwnership(collectorapi.TemporaryError(
		fmt.Errorf("resolved-sensitive-control: %w", jobmgr.ErrProcessAttemptStopped),
	))

	redacted := redactResolvedLifecycleError(err)

	require.Contains(t, redacted.Error(), "redacted")
	require.NotContains(t, redacted.Error(), "resolved-sensitive")
	require.ErrorIs(t, redacted, jobmgr.ErrProcessAttemptStopped)
	require.True(t, jobmgr.ContainsOnlyErrorLeaves(
		redacted,
		jobmgr.ErrProcessAttemptRetired,
		jobmgr.ErrProcessAttemptStopped,
	))
	require.True(t, lifecycle.OwnershipRetained(redacted))
	require.Equal(t, collectorapi.LifecycleErrorTemporary, collectorapi.ClassifyLifecycleError(redacted))
}

type sensitiveConfigFactoryScope struct {
	resolveErr error
	releaseErr error
}

func (scfs *sensitiveConfigFactoryScope) Resolve(context.Context, string, string) ([]byte, error) {
	if scfs.resolveErr != nil {
		return nil, scfs.resolveErr
	}
	return []byte("resolved"), nil
}

func (scfs *sensitiveConfigFactoryScope) Release(context.Context) error {
	return scfs.releaseErr
}

func testConfigResolver(t testing.TB, resolver *secretresolver.AtomicResolver, scope secretresolver.AtomicScopeAcquirer) *secretconfig.ConfigResolver {
	t.Helper()
	configs, err := secretconfig.NewConfigResolver(resolver, scope)
	require.NoError(t, err)
	return configs
}

func testAtomicResolver(t testing.TB) *secretresolver.AtomicResolver {
	t.Helper()
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	return resolver
}
