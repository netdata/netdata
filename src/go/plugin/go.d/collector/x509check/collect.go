// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"crypto/x509"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"

	"github.com/cloudflare/cfssl/revoke"
)

func (c *Collector) collect() (map[string]int64, error) {
	certs, err := c.prov.certificates()
	if err != nil {
		return nil, err
	}

	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificate was provided by '%s'", c.Config.Source)
	}

	mx := make(map[string]int64)

	if err := c.collectCertificates(mx, certs); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectCertificates(mx map[string]int64, certs []*x509.Certificate) error {
	for i, cert := range certs {
		cn := cert.Subject.CommonName

		if !c.seenCerts[cn] {
			c.seenCerts[cn] = true
			c.addCertCharts(cn, i)
		}

		px := fmt.Sprintf("cert_depth%d_", i)

		mx[px+"expiry"] = int64(time.Until(cert.NotAfter).Seconds())

		if i == 0 && c.CheckRevocation {
			func() {
				rev, ok, err := revoke.VerifyCertificateError(cert)
				if err != nil {
					c.Debug(err)
					return
				}
				if !ok {
					return
				}
				mx[px+"revoked"] = metrix.Bool(rev)
				mx[px+"not_revoked"] = metrix.Bool(!rev)
			}()
		}

		if !c.CheckFullChain {
			break
		}
	}

	return nil
}
