// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns_recursor

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
	_, err := c.scrapeStatistics(context.Background())
	require.ErrorContains(t, err, "bearer token file")
}
