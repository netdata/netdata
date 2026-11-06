// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudflare/cfssl/revoke"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

func TestCollector_CollectFromSource(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	cert := newTestCertificate(t, "localhost", now.Add(24*time.Hour))
	certFile := writePEM(t, cert.Leaf)

	for name, source := range map[string]func(t *testing.T) string{
		"https": func(t *testing.T) string { return "https://" + serveTLS(t, cert) },
		"tcp":   func(t *testing.T) string { return "tcp://" + serveTLS(t, cert) },
		"smtp":  func(t *testing.T) string { return "smtp://" + serveSMTP(t, cert) },
		"file":  func(t *testing.T) string { return "file://" + certFile },
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.Source = source(t)
			// The CA option reads through the privileged file helper, which tests cannot run.
			c.InsecureSkipVerify = true
			c.CheckRevocation = true
			c.now = func() time.Time { return now }

			require.NoError(t, c.Init(t.Context()))
			require.NoError(t, c.Check(t.Context()))
			require.NoError(t, collectCycle(t, c))

			// The certificate has no CRL or OCSP endpoints, so the revocation library reports it as not revoked.
			assert.Equal(t, map[string]float64{
				"expiry/0/":                       86400,
				"revocation_status/0/not_revoked": 1,
				"revocation_status/0/revoked":     0,
			}, readMetrics(c))
		})
	}
}

func TestCollector_CheckIsBounded(t *testing.T) {
	for name, test := range map[string]struct {
		scheme      string
		timeout     time.Duration
		cancelAfter time.Duration
	}{
		"tls handshake timeout":  {scheme: "tcp", timeout: 100 * time.Millisecond},
		"smtp exchange timeout":  {scheme: "smtp", timeout: 100 * time.Millisecond},
		"tls handshake canceled": {scheme: "tcp", timeout: time.Minute, cancelAfter: 100 * time.Millisecond},
		"smtp exchange canceled": {scheme: "smtp", timeout: time.Minute, cancelAfter: 100 * time.Millisecond},
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.Source = test.scheme + "://" + listenSilently(t)
			c.Timeout = confopt.Duration(test.timeout)
			require.NoError(t, c.Init(t.Context()))

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if test.cancelAfter > 0 {
				time.AfterFunc(test.cancelAfter, cancel)
			}
			done := make(chan error, 1)
			go func() { done <- c.Check(ctx) }()

			select {
			case err := <-done:
				assert.Error(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("Check is still blocked on a peer that never answers")
			}
		})
	}
}

func TestRevocationRequestsAreBounded(t *testing.T) {
	// The revocation library sends its CRL, OCSP and issuer requests through this process-wide client.
	require.NotNil(t, revoke.HTTPClient)
	assert.Positive(t, revoke.HTTPClient.Timeout)
}

// newTestCertificate returns a self-signed server certificate.
func newTestCertificate(t *testing.T, commonName string, notAfter time.Time) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:   notAfter.Add(-48 * time.Hour),
		NotAfter:    notAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
		Leaf:        leaf,
	}
}

// writePEM writes cert to a temporary PEM file and returns its path.
func writePEM(t *testing.T, cert *x509.Certificate) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "certificate.pem")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}), 0600))
	return path
}

// serveTLS completes a TLS handshake presenting cert on every connection and returns the listener address.
func serveTLS(t *testing.T, cert tls.Certificate) string {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go acceptEach(ln, func(conn net.Conn) { _ = conn.(*tls.Conn).Handshake() })
	return ln.Addr().String()
}

// serveSMTP answers every connection as an SMTP server that offers STARTTLS with cert.
func serveSMTP(t *testing.T, cert tls.Certificate) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	go acceptEach(ln, func(conn net.Conn) {
		text := textproto.NewConn(conn)
		_ = text.PrintfLine("220 localhost ESMTP")
		for {
			line, err := text.ReadLine()
			if err != nil {
				return
			}
			switch command, _, _ := strings.Cut(line, " "); strings.ToUpper(command) {
			case "EHLO":
				_ = text.PrintfLine("250-localhost")
				_ = text.PrintfLine("250 STARTTLS")
			case "STARTTLS":
				_ = text.PrintfLine("220 ready")
				tlsConn := tls.Server(conn, tlsConfig)
				if tlsConn.Handshake() != nil {
					return
				}
				text = textproto.NewConn(tlsConn)
			case "QUIT":
				_ = text.PrintfLine("221 bye")
				return
			default:
				_ = text.PrintfLine("502 unsupported")
			}
		}
	})
	return ln.Addr().String()
}

// listenSilently accepts connections and never writes to them.
func listenSilently(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })

	go acceptEach(ln, func(net.Conn) { <-stop })
	return ln.Addr().String()
}

// acceptEach serves every accepted connection in its own goroutine and closes it afterwards.
func acceptEach(ln net.Listener, serve func(net.Conn)) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer func() { _ = conn.Close() }()
			serve(conn)
		}()
	}
}
