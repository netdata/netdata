// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretconfig "github.com/netdata/netdata/go/plugins/plugin/agent/secrets"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigModuleFactoryOptionalTest(t *testing.T) {
	failed := errors.New("operational test failed")
	initFailed := errors.New("initialization failed")
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			for name, test := range map[string]struct {
				initErr error
				testErr error
			}{
				"success":                {},
				"failure":                {testErr: failed},
				"unsupported":            {testErr: dyncfg.ErrTestUnsupported},
				"initialization failure": {initErr: initFailed},
			} {
				t.Run(name, func(t *testing.T) {
					var calls []string
					module := &configTestV1{}
					module.InitFunc = func(context.Context) error {
						calls = append(calls, "init")
						return test.initErr
					}
					module.CheckFunc = func(context.Context) error {
						calls = append(calls, "check")
						return errors.New("Check must not run when Test is implemented")
					}
					module.test = func(context.Context) error {
						calls = append(calls, "test")
						return test.testErr
					}
					module.CleanupFunc = func(context.Context) { calls = append(calls, "cleanup") }
					factory := newOperationalTestFactory(t, version, module, &secretconfig.ConfigResolver{})

					err := factory.Test(t.Context(), factoryTestConfig(false))

					if test.initErr != nil {
						require.ErrorIs(t, err, test.initErr)
						assert.Equal(t, []string{"init", "cleanup"}, calls)
					} else {
						if test.testErr != nil {
							require.ErrorIs(t, err, test.testErr)
							assert.Contains(t, err.Error(), "collector test")
						} else {
							require.NoError(t, err)
						}
						assert.Equal(t, []string{"init", "test", "cleanup"}, calls)
					}
				})
			}
		})
	}
}

func TestConfigModuleFactoryOptionalTestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cleaned := false
	module := &configTestV1{}
	module.test = func(testCtx context.Context) error {
		require.Same(t, ctx, testCtx)
		cancel()
		<-testCtx.Done()
		return testCtx.Err()
	}
	module.CleanupFunc = func(cleanupCtx context.Context) {
		assert.NoError(t, cleanupCtx.Err())
		cleaned = true
	}
	factory := newOperationalTestFactory(t, "v1", module, &secretconfig.ConfigResolver{})

	require.ErrorIs(t, factory.Test(ctx, factoryTestConfig(false)), context.Canceled)
	assert.True(t, cleaned)
}

func TestConfigModuleFactoryOptionalTestResolvedSecrets(t *testing.T) {
	const fixture = "synthetic-test-secret"
	resolver, err := secretresolver.NewAtomicResolver(map[string]secretresolver.AtomicProvider{
		"fixture": secretresolver.AtomicProviderFunc(func(context.Context, string) ([]byte, error) {
			return []byte(fixture), nil
		}),
	})
	require.NoError(t, err)
	module := &configTestV1{}
	tested := false
	module.test = func(context.Context) error {
		tested = true
		assert.Equal(t, fixture, module.Config.OptionStr)
		module.Warningf("test log contains %s", module.Config.OptionStr)
		return fmt.Errorf("test error contains %s: %w", module.Config.OptionStr, context.DeadlineExceeded)
	}
	module.CleanupFunc = func(context.Context) { panic("cleanup contains " + module.Config.OptionStr) }
	factory := newOperationalTestFactory(t, "v1", module, testConfigResolver(t, resolver, unavailableStoreScope))
	var logs bytes.Buffer
	factory.logger = logger.NewWithWriter(&logs)
	config := factoryTestConfig(false)
	config["option_str"] = "${fixture:value}"

	err = factory.Test(t.Context(), config)

	require.True(t, tested)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotContains(t, err.Error(), fixture)
	assert.Contains(t, err.Error(), "redacted")
	assert.NotContains(t, logs.String(), fixture)
	assert.Contains(t, logs.String(), "redacted")
}

func TestConfigModuleFactoryOptionalTestNotUsedForConfigurationOrValidation(t *testing.T) {
	module := &configTestV1{}
	module.test = func(context.Context) error {
		t.Fatal("configuration and structural validation must not execute Test")
		return nil
	}
	factory := newOperationalTestFactory(t, "v1", module, &secretconfig.ConfigResolver{})
	require.NoError(t, factory.Validate(t.Context(), factoryTestConfig(false)))
	_, err := factory.Configuration(t.Context(), factoryTestConfig(false))
	require.NoError(t, err)
}

func TestFactoryActivationUsesCheckInsteadOfOptionalTest(t *testing.T) {
	checked, cleaned := false, false
	module := &configTestV1{}
	module.test = func(context.Context) error {
		t.Fatal("activation must not execute the optional Test")
		return nil
	}
	module.CheckFunc = func(context.Context) error {
		checked = true
		return errors.New("activation check rejected the candidate")
	}
	module.CleanupFunc = func(context.Context) { cleaned = true }
	factory, _ := newFactoryTestHarness(t, collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 { return module },
	}, nil)
	permit, tasks := issueTestJobPermit(t, "module_job", 1)

	prepared, failure, err := prepareFactoryTestCandidate(t.Context(), factory, factoryTestConfig(false),
		lifecycle.ResourceIdentity{ID: "module_job", Generation: 1}, permit)

	require.NoError(t, err)
	require.ErrorContains(t, failure, "activation check rejected the candidate")
	assert.False(t, prepared.Valid())
	require.NoError(t, permit.AbortUnused())
	assert.True(t, checked)
	assert.True(t, cleaned)
	assert.Equal(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func newOperationalTestFactory(t *testing.T, version string, module *configTestV1, configs *secretconfig.ConfigResolver) *ConfigModuleFactory {
	t.Helper()
	creator := collectorapi.Creator{}
	if version == "v2" {
		creator.CreateV2 = func() collectorapi.CollectorV2 {
			return &configTestV2{MockCollectorV1: module.MockCollectorV1, test: module.test}
		}
	} else {
		creator.Create = func() collectorapi.CollectorV1 { return module }
	}
	factory, err := NewConfigModuleFactory(ConfigModuleFactoryConfig{
		Modules: collectorapi.Registry{"module": creator},
		Configs: configs,
	})
	require.NoError(t, err)
	return factory
}

type configTestV1 struct {
	collectorapi.MockCollectorV1 `yaml:",inline"`
	test                         func(context.Context) error
}

func (c *configTestV1) Test(ctx context.Context) error { return c.test(ctx) }

type configTestV2 struct {
	collectorapi.MockCollectorV1 `yaml:",inline"`
	test                         func(context.Context) error
}

func (c *configTestV2) Test(ctx context.Context) error   { return c.test(ctx) }
func (*configTestV2) Collect(context.Context) error      { return nil }
func (*configTestV2) MetricStore() metrix.CollectorStore { return nil }
