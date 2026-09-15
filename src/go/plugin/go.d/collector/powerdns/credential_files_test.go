// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns

import (
	"context"
	"io/fs"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/credentialfiletest"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/stretchr/testify/require"
)

func TestMissingBearerFileStopsRequest(t *testing.T) {
	c := New()
	c.BearerTokenFile = filepath.Join(t.TempDir(), "missing-token")
	c.SetCredentialFiles(credentialfiletest.New(t))
	c.httpClient = web.WrapHTTPClient(&http.Client{}, c.CredentialFiles())
	_, err := c.scrapeStatistics(context.Background())
	require.ErrorIs(t, err, fs.ErrNotExist)
}
