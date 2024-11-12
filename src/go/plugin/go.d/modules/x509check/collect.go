// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"crypto/x509"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"

	"github.com/cloudflare/cfssl/revoke"
)

func (x *X509Check) collect() (map[string]int64, error) {
	certs, err := x.prov.certificates()
	if err != nil {
		return nil, err
	}

	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificate was provided by '%s'", x.Config.Source)
	}

	mx := make(map[string]int64)

	if err := x.collectCertificates(mx, certs); err != nil {
		return nil, err
	}

	return mx, nil
}

func (x *X509Check) collectCertificates(mx map[string]int64, certs []*x509.Certificate) error {
	for i, cert := range certs {
		cn := cert.Subject.CommonName

		if !x.seenCerts[cn] {
			x.seenCerts[cn] = true
			x.addCertCharts(cn, i)
		}

		px := fmt.Sprintf("cert_depth%d_", i)

		expiry := int64(time.Until(cert.NotAfter).Seconds())

		mx[px+"expiry"] = expiry

		if i == 0 && x.CheckRevocation {
			rev, ok, err := revoke.VerifyCertificateError(certs[0])
			if err != nil {
				x.Debug(err)
				continue
			}
			if !ok {
				continue
			}
			mx[px+"revoked"] = metrix.Bool(rev)
			mx[px+"not_revoked"] = metrix.Bool(!rev)
		}

		if !x.CheckFullChain {
			break
		}
	}

	return nil
}
