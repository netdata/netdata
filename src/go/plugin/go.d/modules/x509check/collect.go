// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"crypto/x509"
	"fmt"
	"time"

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

	x.collectExpiration(mx, certs)
	if x.CheckRevocation {
		x.collectRevocation(mx, certs)
	}

	return mx, nil
}

func (x *X509Check) collectExpiration(mx map[string]int64, certs []*x509.Certificate) {
	expiry := time.Until(certs[0].NotAfter).Seconds()
	mx["expiry"] = int64(expiry)
	mx["days_until_expiration_warning"] = x.DaysUntilWarn
	mx["days_until_expiration_critical"] = x.DaysUntilCritical

}

func (x *X509Check) collectRevocation(mx map[string]int64, certs []*x509.Certificate) {
	rev, ok, err := revoke.VerifyCertificateError(certs[0])
	if err != nil {
		x.Debug(err)
	}
	if !ok {
		return
	}

	mx["revoked"] = 0
	mx["not_revoked"] = 0

	if rev {
		mx["revoked"] = 1
	} else {
		mx["not_revoked"] = 1
	}
}
