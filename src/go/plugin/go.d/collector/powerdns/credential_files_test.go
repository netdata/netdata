// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns

import (
	"context"
	"github.com/netdata/netdata/go/plugins/pkg/credentialfiletest"
	"github.com/stretchr/testify/require"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestMissingBearerFileStopsRequest(t *testing.T) {
	c := New()
	c.BearerTokenFile = filepath.Join(t.TempDir(), "missing-token")
	c.SetCredentialFiles(credentialfiletest.New(t))
	_, err := c.scrapeStatistics(context.Background())
	require.ErrorIs(t, err, fs.ErrNotExist)
}
