// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"maps"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"golang.org/x/crypto/ocsp"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

// revocationRequestTimeout bounds each CRL, OCSP and issuer request of a revocation check.
const revocationRequestTimeout = 10 * time.Second

// revocationChecker checks a certificate against the CRLs and then the OCSP responders it lists. The issuer
// certificate it lists is downloaded to verify CRL signatures and to build OCSP requests.
type revocationChecker struct {
	client *http.Client
	// crls holds, per CRL URL, the verdict for one certificate serial until the CRL's next update.
	crls map[string]crlVerdict
}

type crlVerdict struct {
	serial     *big.Int
	revoked    bool
	nextUpdate time.Time
}

func newRevocationChecker() *revocationChecker {
	return &revocationChecker{
		client: web.NewHTTPClientWithTransport(web.ClientConfig{
			Timeout: confopt.Duration(revocationRequestTimeout),
		}, nil),
		crls: make(map[string]crlVerdict),
	}
}

// status reports whether cert is revoked. A certificate outside its validity period, or a CRL that cannot be
// downloaded, parsed or verified, is an error. ok is false without an error when no OCSP responder gives a usable
// answer or the issuer needed for the OCSP request is unavailable. Any OCSP status other than good counts as revoked.
func (rc *revocationChecker) status(
	ctx context.Context,
	cert *x509.Certificate,
	now time.Time,
) (revoked, ok bool, err error) {
	if !now.Before(cert.NotAfter) {
		return false, false, fmt.Errorf("certificate expired at %s", cert.NotAfter)
	}
	if !now.After(cert.NotBefore) {
		return false, false, fmt.Errorf("certificate is not valid until %s", cert.NotBefore)
	}

	issuer := sync.OnceValues(func() (*x509.Certificate, error) { return rc.fetchIssuer(ctx, cert) })

	maps.DeleteFunc(rc.crls, func(_ string, v crlVerdict) bool { return !now.Before(v.nextUpdate) })
	for _, crlURL := range cert.CRLDistributionPoints {
		if isLDAPURL(crlURL) {
			continue
		}
		revoked, err := rc.crlStatus(ctx, cert, crlURL, issuer, now)
		if err != nil {
			return false, false, err
		}
		if revoked {
			return true, true, nil
		}
	}

	if len(cert.OCSPServer) == 0 {
		return false, true, nil
	}
	return rc.ocspStatus(ctx, cert, issuer)
}

func (rc *revocationChecker) crlStatus(
	ctx context.Context,
	cert *x509.Certificate,
	crlURL string,
	issuer func() (*x509.Certificate, error),
	now time.Time,
) (bool, error) {
	if v, ok := rc.crls[crlURL]; ok && v.serial.Cmp(cert.SerialNumber) == 0 {
		return v.revoked, nil
	}

	body, err := rc.get(ctx, crlURL)
	if err != nil {
		return false, err
	}
	crl, err := parseCRL(body)
	if err != nil {
		return false, fmt.Errorf("parse CRL '%s': %w", crlURL, err)
	}
	iss, err := issuer()
	if err != nil {
		return false, err
	}
	// Without the issuer the signature cannot be checked, and the CRL is used as downloaded.
	if iss != nil {
		if err := crl.CheckSignatureFrom(iss); err != nil {
			return false, fmt.Errorf("verify CRL '%s': %w", crlURL, err)
		}
	}

	revoked := slices.ContainsFunc(crl.RevokedCertificateEntries, func(e x509.RevocationListEntry) bool {
		return e.SerialNumber.Cmp(cert.SerialNumber) == 0
	})
	if now.Before(crl.NextUpdate) {
		rc.crls[crlURL] = crlVerdict{
			serial:     cert.SerialNumber,
			revoked:    revoked,
			nextUpdate: crl.NextUpdate,
		}
	}
	return revoked, nil
}

func (rc *revocationChecker) ocspStatus(
	ctx context.Context,
	cert *x509.Certificate,
	issuer func() (*x509.Certificate, error),
) (revoked, ok bool, err error) {
	iss, err := issuer()
	if err != nil || iss == nil {
		return false, false, err
	}
	req, err := ocsp.CreateRequest(cert, iss, &ocsp.RequestOptions{
		Hash: crypto.SHA1,
	})
	if err != nil {
		return false, false, fmt.Errorf("create OCSP request: %w", err)
	}

	// The first responder that gives a usable answer decides; failing responders are skipped.
	for _, server := range cert.OCSPServer {
		resp, err := rc.queryOCSP(ctx, server, req, cert, iss)
		if err != nil {
			continue
		}
		return resp.Status != ocsp.Good, true, nil
	}
	return false, false, nil
}

// queryOCSP sends a short request with GET (RFC 6960 Appendix A.1) and a longer one with POST.
func (rc *revocationChecker) queryOCSP(
	ctx context.Context,
	server string,
	ocspReq []byte,
	cert, issuer *x509.Certificate,
) (*ocsp.Response, error) {
	var body []byte
	var err error
	if len(ocspReq) <= 256 {
		body, err = rc.get(ctx, server+"/"+url.QueryEscape(base64.StdEncoding.EncodeToString(ocspReq)))
	} else {
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, server, bytes.NewReader(ocspReq))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/ocsp-request")
		body, err = rc.do(req)
	}
	if err != nil {
		return nil, err
	}
	return ocsp.ParseResponseForCert(body, cert, issuer)
}

// fetchIssuer returns the first issuer certificate that downloads and parses from the URLs cert lists, or nil.
// Its only error is the context's, so that a cancelled download is not taken for a missing issuer.
func (rc *revocationChecker) fetchIssuer(ctx context.Context, cert *x509.Certificate) (*x509.Certificate, error) {
	for _, issuerURL := range cert.IssuingCertificateURL {
		body, err := rc.get(ctx, issuerURL)
		if err != nil {
			continue
		}
		if issuer, err := parseCertificate(body); err == nil {
			return issuer, nil
		}
	}
	return nil, ctx.Err()
}

func (rc *revocationChecker) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return rc.do(req)
}

// do returns the body of a 200 OK response; its errors name the URL.
func (rc *revocationChecker) do(req *http.Request) ([]byte, error) {
	var body []byte
	err := web.DoHTTP(rc.client).Request(req, func(r io.Reader) (err error) {
		body, err = io.ReadAll(r)
		return err
	})
	return body, err
}

// isLDAPURL reports an LDAP CRL distribution point, which is not fetched.
func isLDAPURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && u.Scheme == "ldap"
}

// parseCRL accepts a DER CRL and, as some servers send, a PEM one.
func parseCRL(data []byte) (*x509.RevocationList, error) {
	if block, _ := pem.Decode(data); block != nil && block.Type == "X509 CRL" {
		data = block.Bytes
	}
	return x509.ParseRevocationList(data)
}

// parseCertificate accepts a DER certificate or a single PEM one.
func parseCertificate(data []byte) (*x509.Certificate, error) {
	if block, rest := pem.Decode(data); block != nil {
		if len(bytes.TrimSpace(rest)) > 0 {
			return nil, errors.New("more than one PEM block")
		}
		data = block.Bytes
	}
	return x509.ParseCertificate(data)
}
