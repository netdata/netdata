// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/smtp"
	"net/url"
	"os"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
)

type provider interface {
	certificates() ([]*x509.Certificate, error)
}

type fromFile struct {
	path string
}

type fromNet struct {
	url       *url.URL
	tlsConfig *tls.Config
	timeout   time.Duration
}

type fromSMTP struct {
	url       *url.URL
	tlsConfig *tls.Config
	timeout   time.Duration
}

func newProvider(config Config) (provider, error) {
	sourceURL, err := url.Parse(config.Source)
	if err != nil {
		return nil, fmt.Errorf("source parse: %v", err)
	}

	tlsCfg, err := tlscfg.NewTLSConfig(config.TLSConfig)
	if err != nil {
		return nil, fmt.Errorf("create tls config: %v", err)
	}

	if tlsCfg == nil {
		tlsCfg = &tls.Config{}
	}
	tlsCfg.ServerName = sourceURL.Hostname()

	switch sourceURL.Scheme {
	case "file":
		return &fromFile{path: sourceURL.Path}, nil
	case "https", "udp", "udp4", "udp6", "tcp", "tcp4", "tcp6":
		if sourceURL.Scheme == "https" {
			sourceURL.Scheme = "tcp"
		}
		return &fromNet{url: sourceURL, tlsConfig: tlsCfg, timeout: config.Timeout.Duration()}, nil
	case "smtp":
		sourceURL.Scheme = "tcp"
		return &fromSMTP{url: sourceURL, tlsConfig: tlsCfg, timeout: config.Timeout.Duration()}, nil
	default:
		return nil, fmt.Errorf("unsupported scheme '%s'", sourceURL)
	}
}

func (f fromFile) certificates() ([]*x509.Certificate, error) {
	content, err := os.ReadFile(f.path)
	if err != nil {
		return nil, fmt.Errorf("error on reading '%s': %v", f.path, err)
	}

	block, _ := pem.Decode(content)
	if block == nil {
		return nil, fmt.Errorf("error on decoding '%s': %v", f.path, err)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("error on parsing certificate '%s': %v", f.path, err)
	}

	return []*x509.Certificate{cert}, nil
}

func (f fromNet) certificates() ([]*x509.Certificate, error) {
	ipConn, err := net.DialTimeout(f.url.Scheme, f.url.Host, f.timeout)
	if err != nil {
		return nil, fmt.Errorf("error on dial to '%s': %v", f.url, err)
	}
	defer func() { _ = ipConn.Close() }()

	conn := tls.Client(ipConn, f.tlsConfig.Clone())
	defer func() { _ = conn.Close() }()
	if err := conn.Handshake(); err != nil {
		return nil, fmt.Errorf("error on SSL handshake with '%s': %v", f.url, err)
	}

	certs := conn.ConnectionState().PeerCertificates
	return certs, nil
}

func (f fromSMTP) certificates() ([]*x509.Certificate, error) {
	ipConn, err := net.DialTimeout(f.url.Scheme, f.url.Host, f.timeout)
	if err != nil {
		return nil, fmt.Errorf("error on dial to '%s': %v", f.url, err)
	}
	defer func() { _ = ipConn.Close() }()

	host, _, _ := net.SplitHostPort(f.url.Host)
	smtpClient, err := smtp.NewClient(ipConn, host)
	if err != nil {
		return nil, fmt.Errorf("error on creating SMTP client: %v", err)
	}
	defer func() { _ = smtpClient.Quit() }()

	err = smtpClient.StartTLS(f.tlsConfig.Clone())
	if err != nil {
		return nil, fmt.Errorf("error on startTLS with '%s': %v", f.url, err)
	}

	conn, ok := smtpClient.TLSConnectionState()
	if !ok {
		return nil, fmt.Errorf("startTLS didn't succeed")
	}
	return conn.PeerCertificates, nil
}
