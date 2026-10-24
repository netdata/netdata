// SPDX-License-Identifier: GPL-3.0-or-later

package client

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSOAPUsesConfiguredCA(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	require.NoError(t, err)
	parsed, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	tlsConfig := &tls.Config{RootCAs: roots}
	cli, err := newSoapClient(context.Background(), Config{URL: "https://localhost/sdk"})
	require.NoError(t, err)
	t.Cleanup(cli.CloseIdleConnections)
	configureSoapTLS(cli, tlsConfig)
	transport, ok := cli.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig.RootCAs)
	_, err = parsed.Verify(x509.VerifyOptions{Roots: transport.TLSClientConfig.RootCAs})
	require.NoError(t, err)
}
