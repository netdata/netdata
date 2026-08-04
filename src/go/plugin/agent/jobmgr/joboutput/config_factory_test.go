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
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
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
					Resolver:   resolver,
					StoreScope: unavailableStoreScope,
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
		Resolver:   resolver,
		StoreScope: unavailableStoreScope,
	})
	require.NoError(t, err)
	config := factoryTestConfig(false)
	config["option_int"] = "${fixture:value}"

	err = factory.Validate(context.Background(), config)
	require.Error(t, err)
	require.NotContains(t, err.Error(), resolvedFixture)
	require.True(t, strings.Contains(err.Error(), "resolved") && strings.Contains(err.Error(), "redacted"))
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
				Resolver:   test.resolver(t),
				StoreScope: test.storeScope,
			})
			require.NoError(t, err)
			config := factoryTestConfig(false)
			config["option_str"] = test.reference
			config["option_int"] = 1

			err = factory.Validate(context.Background(), config)
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
				Resolver:   resolver,
				StoreScope: unavailableStoreScope,
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
		Resolver:   resolver,
		StoreScope: unavailableStoreScope,
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

type sensitiveCodedRetryableError struct{}

func (sensitiveCodedRetryableError) Error() string         { return "resolved-sensitive-fixture" }
func (sensitiveCodedRetryableError) DyncfgCode() int       { return 429 }
func (sensitiveCodedRetryableError) DyncfgRetryable() bool { return true }

func TestResolvedLifecycleRedactionPreservesControlClassifications(t *testing.T) {
	err := lifecycle.RetainOwnership(errors.Join(
		lifecycle.ErrTaskPanic,
		context.Canceled,
		sensitiveCodedRetryableError{},
	))

	redacted := redactResolvedLifecycleError(err)

	require.NotContains(t, redacted.Error(), "resolved-sensitive-fixture")
	require.Contains(t, redacted.Error(), "redacted")
	require.ErrorIs(t, redacted, lifecycle.ErrTaskPanic)
	require.ErrorIs(t, redacted, context.Canceled)
	require.True(t, lifecycle.OwnershipRetained(redacted))
	require.True(t, dyncfg.IsRetryableError(redacted))
	coded, ok := errors.AsType[dyncfg.CodedError](redacted)
	require.True(t, ok)
	require.Equal(t, 429, coded.DyncfgCode())
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
