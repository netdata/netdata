// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestConfigModuleFactoryAdmissionDoesNotResolveReferences(t *testing.T) {
	tests := map[string]struct {
		value   any
		other   any
		source  string
		wantErr bool
	}{
		"provider reference":                {value: "${fixture:value}"},
		"store reference":                   {value: "${store:vault:missing:value}"},
		"mixed scalar":                      {value: "prefix-${fixture:value}"},
		"invalid available sibling":         {value: "${fixture:value}", other: map[string]any{"invalid": true}, wantErr: true},
		"invalid literal":                   {value: map[string]any{"invalid": true}, wantErr: true},
		"malformed reference":               {value: "${store:invalid}", wantErr: true},
		"unknown provider":                  {value: "${unknown:value}", wantErr: true},
		"untrusted reference stays literal": {value: "${fixture:value}", source: confgroup.TypeDiscovered, wantErr: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			providerCalls, scopeCalls, initCalls, cleanupCalls := 0, 0, 0, 0
			resolver, err := secretresolver.NewAtomicResolver(map[string]secretresolver.AtomicProvider{
				"fixture": secretresolver.AtomicProviderFunc(func(context.Context, string) ([]byte, error) {
					providerCalls++
					return nil, errors.New("unavailable provider")
				}),
			})
			require.NoError(t, err)
			factory, err := NewConfigModuleFactory(ConfigModuleFactoryConfig{
				Modules: collectorapi.Registry{"module": {Create: func() collectorapi.CollectorV1 {
					return &collectorapi.MockCollectorV1{
						InitFunc:    func(context.Context) error { initCalls++; return nil },
						CleanupFunc: func(context.Context) { cleanupCalls++ },
					}
				}}},
				Configs: testConfigResolver(t, resolver, func([]string) (secretresolver.AtomicScope, error) {
					scopeCalls++
					return nil, errors.New("unavailable Store")
				}),
			})
			require.NoError(t, err)
			config := factoryTestConfig(false)
			config.SetSourceType(confgroup.TypeDyncfg)
			if test.source != "" {
				config.SetSourceType(test.source)
			}
			config["option_int"] = test.value
			if test.other != nil {
				config["option_str"] = test.other
			}
			err = factory.Validate(context.Background(), config)
			require.Equal(t, test.wantErr, err != nil, "validation error: %v", err)
			require.Zero(t, providerCalls)
			require.Zero(t, scopeCalls)
			require.Zero(t, initCalls)
			require.Equal(t, 1, cleanupCalls)
			require.Equal(t, test.value, config["option_int"], "admission must preserve the raw configuration")
		})
	}
}

type admissionTestCollector struct {
	*collectorapi.MockCollectorV1 `yaml:"-"`
	Values                        []struct {
		Known  int `yaml:"known"`
		Secret int `yaml:"secret"`
	} `yaml:"values"`
	Choices  map[string]int      `yaml:"choices"`
	Duration confopt.Duration    `yaml:"duration"`
	Panic    admissionPanicValue `yaml:"panic"`
}

type admissionPanicValue string

func (*admissionPanicValue) UnmarshalYAML(func(any) error) error {
	panic("sensitive admission fixture")
}

func TestConfigModuleFactoryAdmissionContainsPanics(t *testing.T) {
	for _, phase := range []string{"decode", "cleanup"} {
		t.Run(phase, func(t *testing.T) {
			resolver, err := secretresolver.NewDefaultAtomicResolver()
			require.NoError(t, err)
			cleanupCalls := 0
			factory, err := NewConfigModuleFactory(ConfigModuleFactoryConfig{
				Modules: collectorapi.Registry{"module": {Create: func() collectorapi.CollectorV1 {
					return &admissionTestCollector{MockCollectorV1: &collectorapi.MockCollectorV1{
						CleanupFunc: func(context.Context) {
							cleanupCalls++
							if phase == "cleanup" {
								panic("sensitive admission fixture")
							}
						},
					}}
				}}},
				Configs: testConfigResolver(t, resolver, unavailableStoreScope),
			})
			require.NoError(t, err)
			config := factoryTestConfig(false).SetSourceType(confgroup.TypeDyncfg)
			config["choices"] = map[string]string{"secret": "${env:NOT_READ}"}
			if phase == "decode" {
				config["panic"] = "trigger"
			}
			err = factory.Validate(context.Background(), config)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "sensitive admission fixture")
			require.Equal(t, "panic", jobConfigFailure(err, "").Reason)
			require.Equal(t, 1, cleanupCalls)
		})
	}
}

func TestConfigModuleFactoryAdmissionValidatesAvailableNestedFields(t *testing.T) {
	tests := map[string]struct {
		known   any
		values  any
		wantErr bool
	}{
		"defer nested references": {known: 3},
		"invalid nested sibling":  {known: map[string]any{"wrong": true}, wantErr: true},
		"invalid container":       {values: "wrong", wantErr: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			resolver, err := secretresolver.NewDefaultAtomicResolver()
			require.NoError(t, err)
			var module *admissionTestCollector
			factory, err := NewConfigModuleFactory(ConfigModuleFactoryConfig{
				Modules: collectorapi.Registry{"module": {Create: func() collectorapi.CollectorV1 {
					module = &admissionTestCollector{MockCollectorV1: &collectorapi.MockCollectorV1{}}
					return module
				}}},
				Configs: testConfigResolver(t, resolver, unavailableStoreScope),
			})
			require.NoError(t, err)
			config := factoryTestConfig(false).SetSourceType(confgroup.TypeDyncfg)
			config["values"] = []any{map[string]any{"known": test.known, "secret": "${env:NOT_READ}"}}
			if test.values != nil {
				config["values"] = test.values
			}
			config["choices"] = map[string]string{"secret": "${store:vault:missing:key}"}
			config["duration"] = "${env:NOT_READ}"
			err = factory.Validate(context.Background(), config)
			require.Equal(t, test.wantErr, err != nil, "validation error: %v", err)
			require.True(t, module.CleanupDone)
			if !test.wantErr {
				require.Len(t, module.Values, 1)
				require.Equal(t, 3, module.Values[0].Known)
			}
		})
	}
}
