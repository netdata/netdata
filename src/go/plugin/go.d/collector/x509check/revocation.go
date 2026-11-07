// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"golang.org/x/crypto/cryptobyte"
	cryptobyte_asn1 "golang.org/x/crypto/cryptobyte/asn1"
	"golang.org/x/crypto/ocsp"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const (
	// revocationRequestTimeout bounds each CRL, OCSP and issuer request of a revocation check.
	revocationRequestTimeout = 10 * time.Second
	// revocationClockSkew is how far a CRL or OCSP answer may seem outside its validity period, for clocks that
	// differ between the CA and the Agent. It is the default of OpenSSL's `ocsp -validity_period`.
	revocationClockSkew = 5 * time.Minute
)

// revocationChecker checks a certificate against the CRLs and then the OCSP responders it lists. It uses only CRLs and
// OCSP answers from the certificate's issuer, which must have signed the certificate: one the source presented with
// it, or else one downloaded from the issuer URLs the certificate lists.
type revocationChecker struct {
	client *http.Client
	// crls holds CRL verdicts until the CRL's next update. A job can see several certificates behind one source,
	// so a verdict is kept per certificate, which its issuer name and serial identify (RFC 5280 4.1.2.2).
	crls map[crlKey]crlVerdict
}

type crlKey struct {
	url    string
	issuer string
	serial string
}

type crlVerdict struct {
	revoked    bool
	nextUpdate time.Time
}

func newRevocationChecker() *revocationChecker {
	return &revocationChecker{
		client: web.NewHTTPClientWithTransport(web.ClientConfig{
			Timeout: confopt.Duration(revocationRequestTimeout),
		}, nil),
		crls: make(map[crlKey]crlVerdict),
	}
}

// status reports whether the server certificate, chain[0], is revoked; the rest of chain is what the source presented
// with it. A certificate outside its validity period, or a CRL that cannot be downloaded, parsed or verified, is an
// error. ok is false without an error when no issuer that signed the certificate is available, or no OCSP responder
// gives a usable answer. Any OCSP status other than good counts as revoked.
func (rc *revocationChecker) status(
	ctx context.Context,
	chain []*x509.Certificate,
	now time.Time,
) (revoked, ok bool, err error) {
	cert := chain[0]
	// The validity period includes both bounds (RFC 5280 4.1.2.5).
	if now.After(cert.NotAfter) {
		return false, false, fmt.Errorf("certificate expired at %s", cert.NotAfter)
	}
	if now.Before(cert.NotBefore) {
		return false, false, fmt.Errorf("certificate is not valid until %s", cert.NotBefore)
	}

	issuer := sync.OnceValues(func() (*x509.Certificate, error) { return rc.findIssuer(ctx, chain) })

	// The only expiry rule: a verdict without a future next update serves no later check.
	maps.DeleteFunc(rc.crls, func(_ crlKey, v crlVerdict) bool { return !now.Before(v.nextUpdate) })
	for _, crlURL := range cert.CRLDistributionPoints {
		if isLDAPURL(crlURL) {
			continue
		}
		revoked, ok, err := rc.crlStatus(ctx, cert, crlURL, issuer, now)
		// A missing issuer leaves OCSP unusable too.
		if err != nil || !ok || revoked {
			return revoked, ok, err
		}
	}

	if len(cert.OCSPServer) == 0 {
		return false, true, nil
	}
	return rc.ocspStatus(ctx, cert, issuer, now)
}

// crlStatus reports whether the CRL at crlURL lists cert. ok is false without an error when no issuer is available to
// verify the CRL.
func (rc *revocationChecker) crlStatus(
	ctx context.Context,
	cert *x509.Certificate,
	crlURL string,
	issuer func() (*x509.Certificate, error),
	now time.Time,
) (revoked, ok bool, err error) {
	key := crlKey{
		url:    crlURL,
		issuer: string(cert.RawIssuer),
		serial: cert.SerialNumber.String(),
	}
	if v, ok := rc.crls[key]; ok {
		return v.revoked, true, nil
	}

	iss, err := issuer()
	if err != nil || iss == nil {
		return false, false, err
	}
	body, err := rc.get(ctx, crlURL)
	if err != nil {
		return false, false, err
	}
	crl, err := parseCRL(body)
	if err != nil {
		return false, false, fmt.Errorf("parse CRL '%s': %w", crlURL, err)
	}
	if err := verifyCRL(crl, cert, iss, now); err != nil {
		return false, false, fmt.Errorf("verify CRL '%s': %w", crlURL, err)
	}

	revoked = slices.ContainsFunc(crl.RevokedCertificateEntries, func(e x509.RevocationListEntry) bool {
		return e.SerialNumber.Cmp(cert.SerialNumber) == 0
	})
	rc.crls[key] = crlVerdict{
		revoked:    revoked,
		nextUpdate: crl.NextUpdate,
	}
	return revoked, true, nil
}

func (rc *revocationChecker) ocspStatus(
	ctx context.Context,
	cert *x509.Certificate,
	issuer func() (*x509.Certificate, error),
	now time.Time,
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
		if err != nil || verifyOCSPResponse(resp, iss, now) != nil {
			continue
		}
		return resp.Status != ocsp.Good, true, nil
	}
	return false, false, nil
}

// queryOCSP sends a request of up to 256 bytes with GET, encoded into the URL as RFC 6960 Appendix A.1 describes,
// and a longer one with POST.
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

// findIssuer returns the certificate that signed chain[0]: the first one the source presented with it, or else the
// first one downloaded from the issuer URLs chain[0] lists, or nil. A key that verifies the certificate's signature is
// the key of its issuer, so CRLs and OCSP answers it verifies come from that issuer. Its only error is the context's,
// so that a cancelled download is not taken for a missing issuer.
func (rc *revocationChecker) findIssuer(ctx context.Context, chain []*x509.Certificate) (*x509.Certificate, error) {
	cert := chain[0]
	for _, issuer := range chain[1:] {
		if cert.CheckSignatureFrom(issuer) == nil {
			return issuer, nil
		}
	}
	for _, issuerURL := range cert.IssuingCertificateURL {
		body, err := rc.get(ctx, issuerURL)
		if err != nil {
			continue
		}
		if issuer, err := parseCertificate(body); err == nil && cert.CheckSignatureFrom(issuer) == nil {
			return issuer, nil
		}
	}
	return nil, ctx.Err()
}

// verifyCRL checks that crl is a current and complete CRL of cert's issuer that covers cert (RFC 5280 6.3.3).
func verifyCRL(crl *x509.RevocationList, cert, issuer *x509.Certificate, now time.Time) error {
	// The issuer certificate's own subject is not compared: only its key is authenticated, by cert's signature.
	if !bytes.Equal(crl.RawIssuer, cert.RawIssuer) {
		return fmt.Errorf("issued by '%s', not by the certificate's issuer '%s'", crl.Issuer, cert.Issuer)
	}
	// Only the signature is checked, so a private CA whose certificate lacks the CRL-signing key usage still verifies.
	if err := issuer.CheckSignature(crl.SignatureAlgorithm, crl.RawTBSRevocationList, crl.Signature); err != nil {
		return err
	}
	if err := checkUpdateTimes(crl.ThisUpdate, crl.NextUpdate, now); err != nil {
		return err
	}
	return checkCRLScope(crl, cert)
}

// verifyOCSPResponse checks what ocsp.ParseResponseForCert, which verifies the signature, leaves out: the answer must
// be current (RFC 6960 3.2), and a delegated responder that signed it must be authorized for OCSP signing by the
// issuer and within its validity period (RFC 6960 4.2.2.2).
func verifyOCSPResponse(resp *ocsp.Response, issuer *x509.Certificate, now time.Time) error {
	if err := checkUpdateTimes(resp.ThisUpdate, resp.NextUpdate, now); err != nil {
		return err
	}
	// An embedded certificate with the issuer's key is the issuer's own.
	responder := resp.Certificate
	if responder == nil || bytes.Equal(responder.RawSubjectPublicKeyInfo, issuer.RawSubjectPublicKeyInfo) {
		return nil
	}
	if !slices.Contains(responder.ExtKeyUsage, x509.ExtKeyUsageOCSPSigning) {
		return errors.New("responder certificate is not authorized for OCSP signing")
	}
	if now.Before(responder.NotBefore) || now.After(responder.NotAfter) {
		return errors.New("responder certificate is outside its validity period")
	}
	return nil
}

// checkUpdateTimes checks that now falls between a CRL's or an OCSP answer's this-update and next-update times, within
// revocationClockSkew. Without a next update, newer information may be available at any time (RFC 6960 4.2.2.1), and
// only the this-update time is checked.
func checkUpdateTimes(thisUpdate, nextUpdate, now time.Time) error {
	if thisUpdate.After(now.Add(revocationClockSkew)) {
		return fmt.Errorf("not valid until %s", thisUpdate)
	}
	if !nextUpdate.IsZero() && now.After(nextUpdate.Add(revocationClockSkew)) {
		return fmt.Errorf("outdated since %s", nextUpdate)
	}
	return nil
}

var oidExtensionIssuingDistributionPoint = asn1.ObjectIdentifier{2, 5, 29, 28}

// checkCRLScope checks that crl covers cert. A CRL with a critical extension that is not processed here, or with an
// entry that has one, must not be used (RFC 5280 5.2, 5.3); among them are delta CRLs and the entries of indirect
// CRLs. Only the issuing distribution point is processed: the other extensions a conforming CRL carries are
// non-critical.
func checkCRLScope(crl *x509.RevocationList, cert *x509.Certificate) error {
	for _, ext := range crl.Extensions {
		switch {
		case ext.Id.Equal(oidExtensionIssuingDistributionPoint):
			if err := checkIssuingDistributionPoint(ext.Value, cert); err != nil {
				return err
			}
		case ext.Critical:
			return fmt.Errorf("unsupported critical extension %s", ext.Id)
		}
	}
	for _, entry := range crl.RevokedCertificateEntries {
		for _, ext := range entry.Extensions {
			if ext.Critical {
				return fmt.Errorf("unsupported critical entry extension %s", ext.Id)
			}
		}
	}
	return nil
}

// checkIssuingDistributionPoint checks that a CRL's issuing distribution point extension (RFC 5280 5.2.5) does not
// limit the CRL to other certificates than cert (RFC 5280 6.3.3 (b)(2)). A distribution point name must name one of
// the CRL URLs cert lists, so that another partition of the issuer's CRLs is not taken for cert's.
//
//	IssuingDistributionPoint ::= SEQUENCE {
//	     distributionPoint          [0] DistributionPointName OPTIONAL,
//	     onlyContainsUserCerts      [1] BOOLEAN DEFAULT FALSE,
//	     onlyContainsCACerts        [2] BOOLEAN DEFAULT FALSE,
//	     onlySomeReasons            [3] ReasonFlags OPTIONAL,
//	     indirectCRL                [4] BOOLEAN DEFAULT FALSE,
//	     onlyContainsAttributeCerts [5] BOOLEAN DEFAULT FALSE }
//
//	DistributionPointName ::= CHOICE {
//	     fullName                [0] GeneralNames,
//	     nameRelativeToCRLIssuer [1] RelativeDistinguishedName }
func checkIssuingDistributionPoint(der []byte, cert *x509.Certificate) error {
	idp := cryptobyte.String(der)
	if !idp.ReadASN1(&idp, cryptobyte_asn1.SEQUENCE) {
		return errMalformedIDP
	}

	var dpName cryptobyte.String
	var hasDPName bool
	if !idp.ReadOptionalASN1(&dpName, &hasDPName, cryptobyte_asn1.Tag(0).Constructed().ContextSpecific()) {
		return errMalformedIDP
	}
	if hasDPName && !namesCRLURL(dpName, cert.CRLDistributionPoints) {
		return errors.New("issuing distribution point names none of the certificate's CRL URLs")
	}

	const (
		onlyContainsUserCerts = iota + 1
		onlyContainsCACerts
		onlySomeReasons
		indirectCRL
		onlyContainsAttributeCerts
	)
	// The fields are implicitly tagged, and DER encodes a BOOLEAN as a single 0x00 or 0xFF octet.
	var flags [onlyContainsAttributeCerts + 1]bool
	for tag := onlyContainsUserCerts; tag < len(flags); tag++ {
		var value cryptobyte.String
		var present bool
		if !idp.ReadOptionalASN1(&value, &present, cryptobyte_asn1.Tag(tag).ContextSpecific()) {
			return errMalformedIDP
		}
		switch {
		case !present:
		case tag == onlySomeReasons: // a BIT STRING
			flags[tag] = true
		case len(value) == 1 && (value[0] == 0x00 || value[0] == 0xff):
			flags[tag] = value[0] == 0xff
		default:
			return errMalformedIDP
		}
	}
	if !idp.Empty() {
		return errMalformedIDP
	}

	switch {
	case flags[onlyContainsUserCerts] && cert.IsCA:
		return errors.New("CRL covers only end-entity certificates")
	case flags[onlyContainsCACerts] && !cert.IsCA:
		return errors.New("CRL covers only CA certificates")
	case flags[onlySomeReasons]:
		return errors.New("CRL covers only some revocation reasons")
	case flags[indirectCRL]:
		return errors.New("indirect CRL")
	case flags[onlyContainsAttributeCerts]:
		return errors.New("CRL covers only attribute certificates")
	}
	return nil
}

var errMalformedIDP = errors.New("malformed issuing distribution point extension")

// namesCRLURL reports whether a distribution point name holds one of urls. Only a full name can: a name relative to
// the CRL issuer is a directory name.
func namesCRLURL(dpName cryptobyte.String, urls []string) bool {
	var fullName cryptobyte.String
	if !dpName.ReadASN1(&fullName, cryptobyte_asn1.Tag(0).Constructed().ContextSpecific()) {
		return false
	}
	for !fullName.Empty() {
		var name cryptobyte.String
		var tag cryptobyte_asn1.Tag
		if !fullName.ReadAnyASN1(&name, &tag) {
			return false
		}
		// uniformResourceIdentifier [6] IA5String
		if tag == cryptobyte_asn1.Tag(6).ContextSpecific() && slices.Contains(urls, string(name)) {
			return true
		}
	}
	return false
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
