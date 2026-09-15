// SPDX-License-Identifier: GPL-3.0-or-later

package nginxplus

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
	_, err := c.queryAPIVersion(context.Background())
	require.ErrorContains(t, err, "bearer token file")
	require.ErrorContains(t, c.queryAvailableEndpoints(context.Background()), "bearer token file")
	metrics := &nginxMetrics{}
	require.NotPanics(t, func() { c.queryNginxInfo(context.Background(), metrics) })
	require.Empty(t, metrics)
}
