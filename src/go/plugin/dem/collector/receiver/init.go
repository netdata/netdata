// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"

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
	if err := c.Receiver.Validate(); err != nil {
		return err
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
	c.geo = geoip.New(c.GeoIPDB, c.geoPaths)
	return nil
}
