// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileCertificateErrorDoesNotExposeContent(t *testing.T) {
	const marker = "private_certificate_marker"
	filename := filepath.Join(t.TempDir(), "certificate.pem")
	require.NoError(t, os.WriteFile(filename, collecttest.CertificateWithInvalidURI(t, marker), 0600))
	c := New()
	c.Source = "file://" + filename
	require.NoError(t, c.Init(t.Context()))
	err := c.Check(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), filename)
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		assert.NotContains(t, cause.Error(), marker)
	}
}
