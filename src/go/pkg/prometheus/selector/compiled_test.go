// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCompiled(t *testing.T) {
	compiled, err := ParseCompiled(`http_requests_total{method="get",code=~"2.."}`)
	require.NoError(t, err)
	assert.Equal(t, Meta{
		MetricNames:          []string{"http_requests_total"},
		ConstrainedLabelKeys: []string{"code", "method"},
	}, compiled.Meta())
	assert.True(t, compiled.Matches(labels.FromStrings(
		"__name__", "http_requests_total",
		"code", "200",
		"method", "get",
	)))
	assert.False(t, compiled.Matches(labels.FromStrings(
		"__name__", "http_requests_total",
		"code", "500",
		"method", "get",
	)))

	meta := compiled.Meta()
	meta.MetricNames[0] = "changed"
	assert.Equal(t, []string{"http_requests_total"}, compiled.Meta().MetricNames)
}

func TestParseCompiledWildcardNameHasNoExactIndexKey(t *testing.T) {
	compiled, err := ParseCompiled(`http_*{method="get"}`)
	require.NoError(t, err)
	assert.Empty(t, compiled.Meta().MetricNames)
	assert.Equal(t, []string{"method"}, compiled.Meta().ConstrainedLabelKeys)
}
