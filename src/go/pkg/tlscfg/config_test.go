// SPDX-License-Identifier: GPL-3.0-or-later

package tlscfg

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	"github.com/stretchr/testify/require"
)

func TestNewTLSConfig(t *testing.T) {
	certPEM, keyPEM := newTestKeyPair(t)

	t.Run("returns nil when TLS is not configured", func(t *testing.T) {
		cfg, err := NewTLSConfig(TLSConfig{})

		require.NoError(t, err)
		require.Nil(t, cfg)
	})

	t.Run("loads a CA and client keypair", func(t *testing.T) {
		dir := t.TempDir()
		caPath := writeTLSFile(t, dir, "ca.pem", certPEM)
		certPath := writeTLSFile(t, dir, "cert.pem", certPEM)
		keyPath := writeTLSFile(t, dir, "key.pem", keyPEM)

		cfg, err := NewTLSConfig(TLSConfig{
			TLSCA:   caPath,
			TLSCert: certPath,
			TLSKey:  keyPath,
		})

		require.NoError(t, err)
		require.NotNil(t, cfg.RootCAs)
		require.Len(t, cfg.Certificates, 1)
	})

	for _, tc := range []struct {
		name string
		file string
	}{
		{name: "CA", file: "ca"},
		{name: "certificate", file: "cert"},
		{name: "key", file: "key"},
	} {
		t.Run("accepts "+tc.name+" at the limit", func(t *testing.T) {
			dir := t.TempDir()
			ca := certPEM
			cert := certPEM
			key := keyPEM
			switch tc.file {
			case "ca":
				ca = padToLimit(t, ca)
			case "cert":
				cert = padToLimit(t, cert)
			case "key":
				key = padToLimit(t, key)
			}

			_, err := NewTLSConfig(TLSConfig{
				TLSCA:   writeTLSFile(t, dir, "ca.pem", ca),
				TLSCert: writeTLSFile(t, dir, "cert.pem", cert),
				TLSKey:  writeTLSFile(t, dir, "key.pem", key),
			})

			require.NoError(t, err)
		})

		t.Run("rejects "+tc.name+" over the limit", func(t *testing.T) {
			dir := t.TempDir()
			ca := certPEM
			cert := certPEM
			key := keyPEM
			tooLarge := make([]byte, safefile.MaxSize+1)
			switch tc.file {
			case "ca":
				ca = tooLarge
			case "cert":
				cert = tooLarge
			case "key":
				key = tooLarge
			}

			_, err := NewTLSConfig(TLSConfig{
				TLSCA:   writeTLSFile(t, dir, "ca.pem", ca),
				TLSCert: writeTLSFile(t, dir, "cert.pem", cert),
				TLSKey:  writeTLSFile(t, dir, "key.pem", key),
			})

			require.ErrorIs(t, err, ErrTLSFile)
			require.ErrorIs(t, err, safefile.ErrFile)
			require.ErrorIs(t, err, safefile.ErrTooLarge)
		})
	}

	t.Run("rejects a non-regular TLS file", func(t *testing.T) {
		_, err := NewTLSConfig(TLSConfig{TLSCA: t.TempDir()})

		require.ErrorIs(t, err, ErrTLSFile)
		require.ErrorIs(t, err, safefile.ErrFile)
		require.ErrorIs(t, err, safefile.ErrNotRegular)
	})

	t.Run("classifies invalid PEM as a TLS file error", func(t *testing.T) {
		path := writeTLSFile(t, t.TempDir(), "ca.pem", []byte("not PEM"))

		_, err := NewTLSConfig(TLSConfig{TLSCA: path})

		require.ErrorIs(t, err, ErrTLSFile)
		require.NotErrorIs(t, err, safefile.ErrFile)
	})
}

func newTestKeyPair(t *testing.T) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

func writeTLSFile(t *testing.T, dir, name string, value []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, value, 0o600))
	return path
}

func padToLimit(t *testing.T, value []byte) []byte {
	t.Helper()
	require.LessOrEqual(t, int64(len(value)), safefile.MaxSize)
	out := make([]byte, safefile.MaxSize)
	copy(out, value)
	for i := len(value); i < len(out); i++ {
		out[i] = ' '
	}
	return out
}
