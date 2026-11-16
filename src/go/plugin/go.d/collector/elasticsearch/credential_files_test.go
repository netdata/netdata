// SPDX-License-Identifier: GPL-3.0-or-later

package elasticsearch

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMissingBearerFileStopsScrapes(t *testing.T) {
	c := New()
	c.BearerTokenFile = filepath.Join(t.TempDir(), "missing-token")

	c.httpClient = &http.Client{}
	metrics := &esMetrics{}
	require.NotPanics(t, func() {
		c.scrapeNodesStats(context.Background(), metrics)
		c.scrapeClusterHealth(context.Background(), metrics)
		c.scrapeClusterStats(context.Background(), metrics)
		c.scrapeLocalIndicesStats(context.Background(), metrics)
	})
	require.Empty(t, metrics)
}
