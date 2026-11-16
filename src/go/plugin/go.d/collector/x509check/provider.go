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
	"slices"
	"time"
)

// certificateProvider fetches the certificates of the source.
type certificateProvider interface {
	certificates(ctx context.Context) (sourceCertificates, error)
}

// sourceCertificates are the certificates a source provides.
type sourceCertificates struct {
	// chain is what the source presents, the server certificate first.
	chain []*x509.Certificate
	// verified holds the chains that TLS verification built from chain up to a trusted CA certificate, which the
	// source need not send, such as a root CA from tls_ca. It is empty when the source is not verified.
	verified [][]*x509.Certificate
}

// issuerCandidates returns the certificates that may have issued the server certificate: the rest of the presented
// chain, and the next certificate of each verified chain.
func (s sourceCertificates) issuerCandidates() []*x509.Certificate {
	candidates := slices.Clone(s.chain[1:])
	for _, chain := range s.verified {
		if len(chain) > 1 {
			candidates = append(candidates, chain[1])
		}
	}
	return candidates
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

func (p *fileProvider) certificates(context.Context) (sourceCertificates, error) {
	content, err := os.ReadFile(p.path)
	if err != nil {
		return sourceCertificates{}, fmt.Errorf("error on reading '%s': %w", p.path, err)
	}

	block, _ := pem.Decode(content)
	if block == nil {
		return sourceCertificates{}, fmt.Errorf(
			"error on decoding '%s': PEM decode failed (not a valid PEM file)",
			p.path,
		)
	}

	// The parser error can quote file content, so it is replaced rather than wrapped.
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return sourceCertificates{}, fmt.Errorf("error on parsing certificate '%s': invalid X.509 certificate", p.path)
	}

	return sourceCertificates{
		chain: []*x509.Certificate{cert},
	}, nil
}

func (p *netProvider) certificates(ctx context.Context) (sourceCertificates, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	conn, err := p.dial(ctx)
	if err != nil {
		return sourceCertificates{}, err
	}

	tlsConn := tls.Client(conn, p.tlsConfig.Clone())
	// Closing the TLS connection sends close_notify after a completed handshake, then closes conn.
	defer func() { _ = tlsConn.Close() }()

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return sourceCertificates{}, fmt.Errorf("error on SSL handshake with '%s': %w", p.address, err)
	}

	return connectionCertificates(tlsConn.ConnectionState()), nil
}

func (p *smtpProvider) certificates(ctx context.Context) (sourceCertificates, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	conn, err := p.dial(ctx)
	if err != nil {
		return sourceCertificates{}, err
	}
	defer func() { _ = conn.Close() }()

	host, _, _ := net.SplitHostPort(p.address)
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return sourceCertificates{}, fmt.Errorf("error on creating SMTP client: %w", err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.StartTLS(p.tlsConfig.Clone()); err != nil {
		return sourceCertificates{}, fmt.Errorf("error on startTLS with '%s': %w", p.address, err)
	}

	state, ok := client.TLSConnectionState()
	if !ok {
		return sourceCertificates{}, errors.New("startTLS didn't succeed")
	}
	return connectionCertificates(state), nil
}

// connectionCertificates returns the certificates of a completed TLS handshake.
func connectionCertificates(state tls.ConnectionState) sourceCertificates {
	return sourceCertificates{
		chain:    state.PeerCertificates,
		verified: state.VerifiedChains,
	}
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
