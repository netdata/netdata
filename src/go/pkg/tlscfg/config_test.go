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
	tooLarge := make([]byte, safefile.MaxSize+1)
	caAtLimit := padToLimit(t, certPEM)
	certAtLimit := padToLimit(t, certPEM)
	keyAtLimit := padToLimit(t, keyPEM)
	tests := map[string]struct {
		config           func(t *testing.T) TLSConfig
		wantNil          bool
		wantRootCAs      bool
		wantCertificates int
		wantErrs         []error
		wantNotErrs      []error
	}{
		"returns nil when TLS is not configured": {
			config:  func(*testing.T) TLSConfig { return TLSConfig{} },
			wantNil: true,
		},
		"loads a CA and client keypair": {
			config: func(t *testing.T) TLSConfig {
				return newTLSFilesConfig(t, certPEM, keyPEM, "", nil)
			},
			wantRootCAs:      true,
			wantCertificates: 1,
		},
		"accepts CA at the limit": {
			config: func(t *testing.T) TLSConfig {
				return newTLSFilesConfig(t, certPEM, keyPEM, "ca", caAtLimit)
			},
			wantRootCAs:      true,
			wantCertificates: 1,
		},
		"accepts certificate at the limit": {
			config: func(t *testing.T) TLSConfig {
				return newTLSFilesConfig(t, certPEM, keyPEM, "cert", certAtLimit)
			},
			wantRootCAs:      true,
			wantCertificates: 1,
		},
		"accepts key at the limit": {
			config: func(t *testing.T) TLSConfig {
				return newTLSFilesConfig(t, certPEM, keyPEM, "key", keyAtLimit)
			},
			wantRootCAs:      true,
			wantCertificates: 1,
		},
		"rejects CA over the limit": {
			config: func(t *testing.T) TLSConfig {
				return newTLSFilesConfig(t, certPEM, keyPEM, "ca", tooLarge)
			},
			wantErrs: []error{ErrTLSFile, safefile.ErrFile, safefile.ErrTooLarge},
		},
		"rejects certificate over the limit": {
			config: func(t *testing.T) TLSConfig {
				return newTLSFilesConfig(t, certPEM, keyPEM, "cert", tooLarge)
			},
			wantErrs: []error{ErrTLSFile, safefile.ErrFile, safefile.ErrTooLarge},
		},
		"rejects key over the limit": {
			config: func(t *testing.T) TLSConfig {
				return newTLSFilesConfig(t, certPEM, keyPEM, "key", tooLarge)
			},
			wantErrs: []error{ErrTLSFile, safefile.ErrFile, safefile.ErrTooLarge},
		},
		"rejects a non-regular TLS file": {
			config: func(t *testing.T) TLSConfig {
				return TLSConfig{TLSCA: t.TempDir()}
			},
			wantErrs: []error{ErrTLSFile, safefile.ErrFile, safefile.ErrNotRegular},
		},
		"classifies invalid PEM as a TLS file error": {
			config: func(t *testing.T) TLSConfig {
				path := writeTLSFile(t, t.TempDir(), "ca.pem", []byte("not PEM"))
				return TLSConfig{TLSCA: path}
			},
			wantErrs:    []error{ErrTLSFile},
			wantNotErrs: []error{safefile.ErrFile},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg, err := NewTLSConfig(tc.config(t))

			if len(tc.wantErrs) > 0 {
				require.Error(t, err)
				for _, wantErr := range tc.wantErrs {
					require.ErrorIs(t, err, wantErr)
				}
				for _, notErr := range tc.wantNotErrs {
					require.NotErrorIs(t, err, notErr)
				}
				return
			}
			require.NoError(t, err)
			if tc.wantNil {
				require.Nil(t, cfg)
				return
			}
			require.NotNil(t, cfg)
			if tc.wantRootCAs {
				require.NotNil(t, cfg.RootCAs)
			}
			require.Len(t, cfg.Certificates, tc.wantCertificates)
		})
	}
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

func newTLSFilesConfig(t *testing.T, certPEM, keyPEM []byte, override string, value []byte) TLSConfig {
	t.Helper()
	ca := certPEM
	cert := certPEM
	key := keyPEM
	switch override {
	case "ca":
		ca = value
	case "cert":
		cert = value
	case "key":
		key = value
	}
	dir := t.TempDir()
	return TLSConfig{
		TLSCA:   writeTLSFile(t, dir, "ca.pem", ca),
		TLSCert: writeTLSFile(t, dir, "cert.pem", cert),
		TLSKey:  writeTLSFile(t, dir, "key.pem", key),
	}
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
