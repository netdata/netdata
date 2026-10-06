// SPDX-License-Identifier: GPL-3.0-or-later

package jetson

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

func TestConfiguration(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, New(), []byte(`{"update_every":5}`), []byte("update_every: 5\n"))
	for name, tc := range map[string]struct {
		interval  int
		wantError bool
	}{
		"default":            {interval: defaultUpdateEvery},
		"slower publication": {interval: 5},
		"zero":               {interval: 0, wantError: true},
		"negative":           {interval: -1, wantError: true},
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.UpdateEvery = tc.interval
			assert.Equal(t, tc.wantError, c.Init(t.Context()) != nil)
		})
	}
}

func TestCheckOnlyResolvesExecutable(t *testing.T) {
	for name, tc := range map[string]struct {
		lookupErr error
		wantPath  string
	}{
		"found":   {wantPath: "/not-executed"},
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
			assert.Equal(t, tc.wantPath, c.tegrastatsPath)
			assert.Nil(t, c.latest.Load())
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
	if runtime.GOOS == "linux" {
		assert.ErrorContains(t, err, "not found in PATH")
	} else {
		assert.ErrorContains(t, err, "requires Linux")
	}
}

func TestArtifacts(t *testing.T) {
	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	collecttest.AssertChartTemplateSchema(t, chartTemplateYAML)
	collecttest.AssertMetadataDocumentsChartTemplate(t, metadata, chartTemplateYAML, nil)
}
