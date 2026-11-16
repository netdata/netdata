// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
)

// initCertificateProvider selects how certificates are fetched from the scheme of the source URL.
func (c *Collector) initCertificateProvider(ctx context.Context) (certificateProvider, error) {
	sourceURL, err := url.Parse(c.Source)
	if err != nil {
		return nil, fmt.Errorf("source parse: %w", err)
	}

	if sourceURL.Scheme == "file" {
		if c.TLSServerName != "" {
			return nil, errors.New("tls_server_name cannot be used with a file:// source")
		}
		return &fileProvider{
			path: sourceURL.Path,
		}, nil
	}

	var network string
	switch sourceURL.Scheme {
	case "https", "smtp":
		network = "tcp"
	case "tcp", "tcp4", "tcp6":
		network = sourceURL.Scheme
	default:
		return nil, fmt.Errorf("unsupported scheme '%s' in source '%s'", sourceURL.Scheme, c.Source)
	}

	tlsConfig, err := tlscfg.NewTLSConfig(ctx, c.TLSConfig)
	if err != nil {
		return nil, fmt.Errorf("create tls config: %w", err)
	}
	if tlsConfig == nil {
		tlsConfig = &tls.Config{}
	}
	// SNI and hostname verification use the server name; the connection still goes to the source address.
	tlsConfig.ServerName = cmp.Or(c.TLSServerName, sourceURL.Hostname())

	ep := endpoint{
		network:   network,
		address:   sourceURL.Host,
		tlsConfig: tlsConfig,
		timeout:   c.Timeout.Duration(),
	}
	if sourceURL.Scheme == "smtp" {
		return &smtpProvider{
			endpoint: ep,
		}, nil
	}
	return &netProvider{
		endpoint: ep,
	}, nil
}
