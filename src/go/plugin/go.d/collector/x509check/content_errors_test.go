// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileCertificateErrorDoesNotExposeContent(t *testing.T) {
	const marker = "private_certificate_marker"
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Unix(1, 0),
		NotAfter:     time.Unix(2, 0),
		URIs:         []*url.URL{{Scheme: "https", Host: "[" + marker}},
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, pub, key)
	require.NoError(t, err)
	filename := filepath.Join(t.TempDir(), "certificate.pem")
	require.NoError(t, os.WriteFile(filename, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600))
	c := New()
	c.Source = "file://" + filename
	require.NoError(t, c.Init(t.Context()))
	err = c.Check(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), filename)
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		assert.NotContains(t, cause.Error(), marker)
	}
}
