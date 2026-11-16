// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func (c *Collector) collect(ctx context.Context) error {
	src, err := c.fetchCertificates(ctx)
	if err != nil {
		return err
	}
	certs := src.chain
	if !c.CheckFullChain {
		certs = certs[:1]
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
			if err := c.collectRevocationStatus(ctx, src, now, labels); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *Collector) fetchCertificates(ctx context.Context) (sourceCertificates, error) {
	src, err := c.provider.certificates(ctx)
	if err != nil {
		// Cancellation ends a network exchange through a connection deadline; report the cancellation itself.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return sourceCertificates{}, ctxErr
		}
		return sourceCertificates{}, err
	}
	if len(src.chain) == 0 {
		return sourceCertificates{}, fmt.Errorf("no certificate was provided by '%s'", c.Source)
	}
	return src, nil
}

// collectRevocationStatus records the status of the server certificate, with a gap when it cannot be determined, and
// the outcome of the check. It returns only the context error.
func (c *Collector) collectRevocationStatus(
	ctx context.Context,
	src sourceCertificates,
	now time.Time,
	labels metrix.LabelSet,
) error {
	revoked, err := c.revocationStatus(ctx, src.chain[0], src.issuerCandidates(), now)
	// A cancelled request reads as an undetermined status; report the cancellation itself.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	if err != nil {
		c.Limit("x509check:revocation", 1, time.Hour).
			Warningf("revocation status of '%s' could not be determined: %s", c.Source, err)
	} else {
		c.metrics.revocationStatus.ObserveStateSet(metrix.StateSetPoint{
			States: map[string]bool{
				"not_revoked": !revoked,
				"revoked":     revoked,
			},
		}, labels)
	}

	// An error that names no outcome leaves a gap rather than a guessed one.
	if outcome := checkOutcome(err); outcome != "" {
		c.metrics.revocationCheckStatus.ObserveStateSet(metrix.StateSetPoint{
			States: map[string]bool{outcome: true},
		}, labels)
	}
	return nil
}
