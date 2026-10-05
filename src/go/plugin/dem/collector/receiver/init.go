// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/geoip"
)

func (c *Collector) Init(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.registry == nil {
		return errors.New("missing runtime routes")
	}
	_, port, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	if _, err := net.DefaultResolver.LookupPort(ctx, "tcp", port); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	if c.MaxBodyBytes <= 0 || c.RateLimit.PerIPPerMin <= 0 || c.RateLimit.PerSitePerSec <= 0 {
		return errors.New("body and rate limits must be positive")
	}
	for _, p := range c.TrustedProxies {
		if _, err := netip.ParsePrefix(p); err != nil {
			if _, err := netip.ParseAddr(p); err != nil {
				return fmt.Errorf("trusted proxy %q: %w", p, err)
			}
		}
	}
	if c.PublicURL != "" {
		u, err := url.Parse(c.PublicURL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil ||
			u.RawQuery != "" ||
			u.Fragment != "" {
			return errors.New("public_url must be an http(s) base URL without credentials, query or fragment")
		}
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return errors.New("tls_cert and tls_key must be set together")
	}
	if c.TLSCert != "" {
		tlsConfig, err := tlscfg.NewTLSConfig(ctx, tlscfg.TLSConfig{
			TLSCert: c.TLSCert,
			TLSKey:  c.TLSKey,
		})
		if err != nil {
			return fmt.Errorf("listener TLS: %w", err)
		}
		tlsConfig.MinVersion = tls.VersionTLS12
		c.tlsConfig = tlsConfig
	}
	resolver, err := geoip.Open(c.GeoIPDB)
	if err != nil && !errors.Is(err, geoip.ErrNotFound) {
		c.Warningf("GeoIP unavailable: %v", err)
	}
	c.geo = resolver
	return nil
}
