// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"time"
)

// certificateProvider fetches the certificate chain of the source, leaf first.
type certificateProvider interface {
	certificates(ctx context.Context) ([]*x509.Certificate, error)
}

// fileProvider reads the first PEM block of a local file.
type fileProvider struct {
	path string
}

// netProvider starts TLS directly on the connection.
type netProvider struct {
	endpoint
}

// smtpProvider upgrades an SMTP session with STARTTLS.
type smtpProvider struct {
	endpoint
}

type endpoint struct {
	network   string
	address   string
	tlsConfig *tls.Config
	timeout   time.Duration
}

func (p *fileProvider) certificates(context.Context) ([]*x509.Certificate, error) {
	content, err := os.ReadFile(p.path)
	if err != nil {
		return nil, fmt.Errorf("error on reading '%s': %w", p.path, err)
	}

	block, _ := pem.Decode(content)
	if block == nil {
		return nil, fmt.Errorf("error on decoding '%s': PEM decode failed (not a valid PEM file)", p.path)
	}

	// The parser error can quote file content, so it is replaced rather than wrapped.
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("error on parsing certificate '%s': invalid X.509 certificate", p.path)
	}

	return []*x509.Certificate{cert}, nil
}

func (p *netProvider) certificates(ctx context.Context) ([]*x509.Certificate, error) {
	ctx, cancel := p.withTimeout(ctx)
	defer cancel()

	conn, err := p.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	tlsConn := tls.Client(conn, p.tlsConfig.Clone())
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("error on SSL handshake with '%s': %w", p.address, err)
	}

	return tlsConn.ConnectionState().PeerCertificates, nil
}

func (p *smtpProvider) certificates(ctx context.Context) ([]*x509.Certificate, error) {
	ctx, cancel := p.withTimeout(ctx)
	defer cancel()

	conn, err := p.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	host, _, _ := net.SplitHostPort(p.address)
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return nil, fmt.Errorf("error on creating SMTP client: %w", err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.StartTLS(p.tlsConfig.Clone()); err != nil {
		return nil, fmt.Errorf("error on startTLS with '%s': %w", p.address, err)
	}

	state, ok := client.TLSConnectionState()
	if !ok {
		return nil, errors.New("startTLS didn't succeed")
	}
	return state.PeerCertificates, nil
}

// withTimeout bounds ctx by the configured timeout; a non-positive timeout leaves ctx unbounded.
func (e endpoint) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if e.timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, e.timeout)
}

// dial connects to the endpoint. ctx also bounds every later exchange on the connection: once it is done,
// pending and future reads and writes fail.
func (e endpoint) dial(ctx context.Context) (net.Conn, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, e.network, e.address)
	if err != nil {
		return nil, fmt.Errorf("error on dial to '%s': %w", e.address, err)
	}
	context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	return conn, nil
}
