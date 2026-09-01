// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChartTemplate(t *testing.T) {
	template := New().ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, template)
	spec, err := charttpl.DecodeYAML([]byte(template))
	require.NoError(t, err)
	require.NoError(t, spec.Validate())
	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)
}

func TestPayloadMismatchPresentationIsModeNeutral(t *testing.T) {
	healthPath := filepath.Join("..", "..", "..", "..", "..", "..", "src", "health", "health.d", "s3check.conf")
	health, err := os.ReadFile(healthPath)
	require.NoError(t, err)
	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)

	assert.NotContains(t, chartTemplateYAML, "replicated payload mismatch")
	assert.NotContains(t, string(health), "payload differs between")
	assert.NotContains(t, string(health), "The destination returned different bytes")
	assert.NotContains(t, string(metadata), "S3 Replicated Payload Mismatch")
	assert.NotContains(t, string(metadata), "The destination returned different bytes")
}
