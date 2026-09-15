// SPDX-License-Identifier: GPL-3.0-or-later

package couchdb

import (
	"context"
	"github.com/netdata/netdata/go/plugins/pkg/credentialfiletest"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestMissingBearerFileStopsRequest(t *testing.T) {
	c := New()
	c.BearerTokenFile = filepath.Join(t.TempDir(), "missing-token")
	c.SetCredentialFiles(credentialfiletest.New(t))
	metrics := &cdbMetrics{}
	require.NotPanics(t, func() {
		c.scrapeNodeStats(context.Background(), metrics)
		c.scrapeSystemStats(context.Background(), metrics)
		c.scrapeActiveTasks(context.Background(), metrics)
		c.scrapeDBStats(context.Background(), metrics)
	})
	require.Empty(t, metrics)
}
