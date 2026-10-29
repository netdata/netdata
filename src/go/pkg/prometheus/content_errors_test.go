// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileScrapeErrorDoesNotExposeContent(t *testing.T) {
	const marker = "private_exposition_marker"
	filename := filepath.Join(t.TempDir(), "metrics")
	require.NoError(t, os.WriteFile(filename, []byte("test_metric "+marker+"\n"), 0600))
	p := New(nil, web.RequestConfig{URL: "file://" + filename})
	_, err := p.ScrapeSeries(t.Context())
	require.Error(t, err)
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		assert.NotContains(t, cause.Error(), marker)
	}
}
