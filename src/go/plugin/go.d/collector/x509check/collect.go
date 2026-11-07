// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"context"
	"crypto/x509"
	"fmt"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func (c *Collector) collect(ctx context.Context) error {
	chain, err := c.fetchCertificates(ctx)
	if err != nil {
		return err
	}
	certs := chain
	if !c.CheckFullChain {
		certs = chain[:1]
	}

	now := c.now()
	for depth, cert := range certs {
		labels := c.metrics.meter.LabelSet(
			metrix.Label{
				Key:   "source",
				Value: c.Source,
			},
			metrix.Label{
				Key:   "common_name",
				Value: cert.Subject.CommonName,
			},
			metrix.Label{
				Key:   "depth",
				Value: strconv.Itoa(depth),
			},
		)

		// Whole seconds, truncated toward zero.
		c.metrics.expiry.Observe(float64(int64(cert.NotAfter.Sub(now).Seconds())), labels)

		if depth == 0 && c.CheckRevocation {
			// The whole chain, which can hold the server certificate's issuer.
			if err := c.collectRevocationStatus(ctx, chain, now, labels); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *Collector) fetchCertificates(ctx context.Context) ([]*x509.Certificate, error) {
	certs, err := c.provider.certificates(ctx)
	if err != nil {
		// Cancellation ends a network exchange through a connection deadline; report the cancellation itself.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificate was provided by '%s'", c.Source)
	}
	return certs, nil
}

// collectRevocationStatus records the status of the server certificate, chain[0], and leaves a gap when it cannot be
// determined. It returns only the context error.
func (c *Collector) collectRevocationStatus(
	ctx context.Context,
	chain []*x509.Certificate,
	now time.Time,
	labels metrix.LabelSet,
) error {
	revoked, ok, err := c.revocationStatus(ctx, chain, now)
	// A cancelled request reads as a failed or undetermined check; report the cancellation itself.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	switch {
	case err != nil:
		c.Limit("x509check:revocation", 1, time.Hour).
			Warningf("revocation status check for '%s' failed: %s", c.Source, err)
	case !ok:
		c.Limit("x509check:revocation", 1, time.Hour).
			Warningf("revocation status of '%s' could not be determined", c.Source)
	default:
		c.metrics.revocationStatus.ObserveStateSet(metrix.StateSetPoint{
			States: map[string]bool{
				"not_revoked": !revoked,
				"revoked":     revoked,
			},
		}, labels)
	}
	return nil
}
