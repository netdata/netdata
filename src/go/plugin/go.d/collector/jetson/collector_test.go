// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

func TestConfiguration(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, New(), []byte(`{"update_every":5}`), []byte("update_every: 5\n"))
}

func TestCheckOnlyResolvesExecutable(t *testing.T) {
	for name, tc := range map[string]struct {
		lookupErr  error
		wantSource bool
	}{
		"found":   {wantSource: true},
		"missing": {lookupErr: errors.New("not installed")},
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			calls := 0
			c.findTegrastats = func() (string, error) {
				calls++
				return "/not-executed", tc.lookupErr
			}
			require.NoError(t, c.Init(t.Context()))
			assert.Zero(t, calls)
			assert.Equal(t, tc.lookupErr, c.Check(t.Context()))
			assert.Equal(t, 1, calls)
			assert.Equal(t, tc.wantSource, c.source != nil)
			_, ok := c.latestSample()
			assert.False(t, ok)
		})
	}
}

func TestCanceledLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	c := New()
	c.findTegrastats = func() (string, error) { t.Fatal("canceled Check performed discovery"); return "", nil }
	assert.ErrorIs(t, c.Check(ctx), context.Canceled)
	assert.ErrorIs(t, c.Collect(ctx), context.Canceled)
}

func TestLookupTegrastats(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := lookupTegrastats()
	assert.EqualError(t, err, "tegrastats executable not found in PATH")
}

func TestArtifacts(t *testing.T) {
	collecttest.AssertChartTemplateSchema(t, chartTemplateYAML)
	spec, err := charttpl.DecodeYAML([]byte(chartTemplateYAML))
	require.NoError(t, err)
	_, err = chartengine.Compile(spec, defaultUpdateEvery)
	require.NoError(t, err)

	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	collecttest.AssertMetadataDocumentsChartTemplate(t, metadata, chartTemplateYAML, nil)
	collecttest.AssertConfigSchemaMatchesMetadataWith(t, "config_schema.json", "metadata.yaml",
		collecttest.ConfigSchemaCheck{
			Defaults: true,
		})
}
