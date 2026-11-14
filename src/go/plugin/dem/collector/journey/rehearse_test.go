// SPDX-License-Identifier: GPL-3.0-or-later

package journey_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjobtest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/journey"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDraftUsesResolvedConfigurationWithoutActivating(t *testing.T) {
	t.Setenv("DEM_TEST_REHEARSAL_SECRET", "synthetic-rehearsal-value")
	hub := syntheticregistry.New()
	var requests []model.Request
	failure := errors.New("fixture execution rejected")
	exec := syntheticjobtest.Executor{
		RehearseFunc: func(ctx context.Context, request model.Request) error {
			assert.Same(t, t.Context(), ctx)
			requests = append(requests, request)
			return failure
		},
	}
	creator := journey.Creator(journey.Dependencies{
		Executor: exec,
		Registry: hub,
	})
	resolver, err := secretresolver.NewDefaultAtomicResolver()
	require.NoError(t, err)
	configs, err := secrets.NewConfigResolver(resolver, func([]string) (secretresolver.AtomicScope, error) {
		return nil, errors.New("fixture does not use named stores")
	})
	require.NoError(t, err)
	factory, err := joboutput.NewConfigModuleFactory(joboutput.ConfigModuleFactoryConfig{
		Modules: collectorapi.Registry{
			"journey": creator,
		},
		Configs: configs,
	})
	require.NoError(t, err)
	config := confgroup.Config{
		"name":         "draft",
		"script":       "export {};",
		"timeout":      42,
		"update_every": 900,
		"secrets":      []any{map[string]any{"name": "DEM_SECRET_TOKEN", "value": "${env:DEM_TEST_REHEARSAL_SECRET}"}},
	}.SetModule("journey").SetSourceType(confgroup.TypeUser)
	require.NoError(t, factory.Validate(t.Context(), config))
	_, err = factory.Configuration(t.Context(), config)
	require.NoError(t, err)
	require.Empty(t, requests)
	err = factory.Test(t.Context(), config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redacted", "existing resolved-secret error policy still applies")
	require.Len(t, requests, 1)
	assert.Equal(t, "draft", requests[0].Name)
	assert.Equal(t, "export {};", requests[0].Script)
	assert.Equal(t, 42*time.Second, requests[0].Timeout)
	assert.Equal(t, map[string]string{"DEM_SECRET_TOKEN": "synthetic-rehearsal-value"}, requests[0].Secrets)
	assert.Empty(t, hub.Snapshot(time.Now()))
	assert.Equal(t, "${env:DEM_TEST_REHEARSAL_SECRET}", config["secrets"].([]any)[0].(map[string]any)["value"])

	c := journey.New(journey.Dependencies{
		Executor: exec,
		Registry: hub,
	})
	c.Name = "checkout"
	c.Script = "export {};"
	require.ErrorContains(t, c.Test(t.Context()), "not initialized")
	require.NoError(t, c.Init(t.Context()))
	require.ErrorIs(t, c.Test(t.Context()), failure)
	require.Len(t, requests, 2)
	assert.Empty(t, hub.Snapshot(time.Now()))
}
