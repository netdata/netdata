// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"crypto/x509"
	"fmt"
	"strconv"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func (c *Collector) certificates() ([]*x509.Certificate, error) {
	certs, err := c.prov.certificates()
	if err != nil {
		return nil, err
	}

	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificate was provided by '%s'", c.Config.Source)
	}

	return certs, nil
}

func (c *Collector) collect() error {
	certs, err := c.certificates()
	if err != nil {
		return err
	}

	for i, cert := range certs {
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
				Value: strconv.Itoa(i),
			},
		)

		c.metrics.expiry.Observe(float64(int64(cert.NotAfter.Sub(c.now()).Seconds())), labels)

		if i == 0 && c.CheckRevocation {
			revoked, ok, err := c.verifyCertificate(cert)
			if err != nil {
				c.Debug(err)
			} else if ok {
				c.metrics.revocation.ObserveStateSet(metrix.StateSetPoint{
					States: map[string]bool{
						"not_revoked": !revoked,
						"revoked":     revoked,
					},
				}, labels)
			}
		}

		if !c.CheckFullChain {
			break
		}
	}

	return nil
}
