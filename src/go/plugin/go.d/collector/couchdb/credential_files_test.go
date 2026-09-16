// SPDX-License-Identifier: GPL-3.0-or-later

package couchdb

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMissingBearerFileStopsRequest(t *testing.T) {
	c := New()
	c.BearerTokenFile = filepath.Join(t.TempDir(), "missing-token")

	c.httpClient = &http.Client{}
	metrics := &cdbMetrics{}
	require.NotPanics(t, func() {
		c.scrapeNodeStats(context.Background(), metrics)
		c.scrapeSystemStats(context.Background(), metrics)
		c.scrapeActiveTasks(context.Background(), metrics)
		c.scrapeDBStats(context.Background(), metrics)
	})
	require.Empty(t, metrics)
}
