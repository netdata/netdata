// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
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
	"strings"
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

// The outcomes of a revocation check, the states of the revocation check status chart. A check that cannot determine
// the status names one of the outcomes other than success.
const (
	checkSuccess          = "success"
	checkRequestFailed    = "request_failed"
	checkInvalidAnswer    = "invalid_answer"
	checkIssuerNotFound   = "issuer_not_found"
	checkNoRevocationInfo = "no_revocation_info"
	checkOutsideValidity  = "outside_validity"
)

var checkOutcomes = []string{
	checkSuccess,
	checkRequestFailed,
	checkInvalidAnswer,
	checkIssuerNotFound,
	checkNoRevocationInfo,
	checkOutsideValidity,
}

// checkError is a revocation status that could not be determined, with the outcome that says why.
type checkError struct {
	outcome string
	err     error
}

func failCheck(outcome string, err error) error {
	return &checkError{outcome: outcome, err: err}
}

func (e *checkError) Error() string { return e.err.Error() }

func (e *checkError) Unwrap() error { return e.err }

// checkOutcome returns the outcome of a check that returned err, or "" for an error that names none.
func checkOutcome(err error) string {
	if err == nil {
		return checkSuccess
	}
	if ce, ok := errors.AsType[*checkError](err); ok {
		return ce.outcome
	}
	return ""
}

// revocationChecker checks a certificate against the CRLs and then the OCSP responders it lists. It uses only CRLs and
// OCSP answers from the certificate's issuer, which must have signed the certificate: one the source presented with
// it or TLS verification found, or else one downloaded from the issuer URLs the certificate lists.
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

// status reports whether cert is revoked; issuers are certificates that may have issued it. Only the HTTP and HTTPS
// URLs cert lists are used. An error means that the status could not be determined, and names why, with an outcome
// (checkOutcome): the certificate is outside its validity period, lists no CRL or OCSP URL to use, no issuer that
// signed it is found, a CRL cannot be downloaded, parsed or verified, or no OCSP responder gives a usable answer. Any
// OCSP status other than good counts as revoked.
func (rc *revocationChecker) status(
	ctx context.Context,
	cert *x509.Certificate,
	issuers []*x509.Certificate,
	now time.Time,
) (bool, error) {
	// The validity period includes both bounds (RFC 5280 4.1.2.5).
	if now.After(cert.NotAfter) {
		return false, failCheck(checkOutsideValidity, fmt.Errorf("certificate expired at %s", cert.NotAfter))
	}
	if now.Before(cert.NotBefore) {
		return false, failCheck(checkOutsideValidity, fmt.Errorf("certificate is not valid until %s", cert.NotBefore))
	}

	issuer := sync.OnceValues(func() (*x509.Certificate, error) { return rc.findIssuer(ctx, cert, issuers) })

	// The only expiry rule: a verdict without a future next update serves no later check.
	maps.DeleteFunc(rc.crls, func(_ crlKey, v crlVerdict) bool { return !now.Before(v.nextUpdate) })
	var crlChecked bool
	for _, crlURL := range cert.CRLDistributionPoints {
		if !isHTTPURL(crlURL) {
			continue
		}
		revoked, err := rc.crlStatus(ctx, cert, crlURL, issuer, now)
		if err != nil || revoked {
			return revoked, err
		}
		crlChecked = true
	}

	if !slices.ContainsFunc(cert.OCSPServer, isHTTPURL) {
		if !crlChecked {
			return false, failCheck(checkNoRevocationInfo,
				errors.New("the certificate lists no HTTP or HTTPS CRL or OCSP URL"))
		}
		return false, nil
	}
	return rc.ocspStatus(ctx, cert, issuer, now)
}

// crlStatus reports whether the CRL at crlURL lists cert.
func (rc *revocationChecker) crlStatus(
	ctx context.Context,
	cert *x509.Certificate,
	crlURL string,
	issuer func() (*x509.Certificate, error),
	now time.Time,
) (bool, error) {
	key := crlKey{
		url:    crlURL,
		issuer: string(cert.RawIssuer),
		serial: cert.SerialNumber.String(),
	}
	if v, ok := rc.crls[key]; ok {
		return v.revoked, nil
	}

	iss, err := issuer()
	if err != nil {
		return false, err
	}
	body, err := rc.get(ctx, crlURL)
	if err != nil {
		return false, err
	}
	crl, err := parseCRL(body)
	if err != nil {
		return false, failCheck(checkInvalidAnswer, fmt.Errorf("parse CRL '%s': %w", crlURL, err))
	}
	if err := verifyCRL(crl, cert, iss, now); err != nil {
		return false, failCheck(checkInvalidAnswer, fmt.Errorf("verify CRL '%s': %w", crlURL, err))
	}

	revoked := slices.ContainsFunc(crl.RevokedCertificateEntries, func(e x509.RevocationListEntry) bool {
		return e.SerialNumber.Cmp(cert.SerialNumber) == 0
	})
	rc.crls[key] = crlVerdict{
		revoked:    revoked,
		nextUpdate: crl.NextUpdate,
	}
	return revoked, nil
}

// ocspStatus reports the status the first responder with a usable answer gives; failing responders are skipped. When
// none gives one, the outcome is a failed request only if every request failed.
func (rc *revocationChecker) ocspStatus(
	ctx context.Context,
	cert *x509.Certificate,
	issuer func() (*x509.Certificate, error),
	now time.Time,
) (bool, error) {
	iss, err := issuer()
	if err != nil {
		return false, err
	}
	// The request names the issuer by cert's issuer field (RFC 6960 4.1.1): only the issuer certificate's key is
	// authenticated, and its subject may be encoded otherwise.
	named := *iss
	named.RawSubject = cert.RawIssuer
	req, err := ocsp.CreateRequest(cert, &named, &ocsp.RequestOptions{
		Hash: crypto.SHA1,
	})
	if err != nil {
		// Not expected: the request takes only the issuer's key from it, which signedBy accepted.
		return false, failCheck(checkIssuerNotFound, fmt.Errorf("create OCSP request: %w", err))
	}

	var reasons []string
	outcome := checkRequestFailed
	for _, server := range cert.OCSPServer {
		if !isHTTPURL(server) {
			continue
		}
		resp, err := rc.queryOCSP(ctx, server, req, cert)
		if err == nil {
			err = verifyOCSPResponse(resp, cert, iss, now)
		}
		if err == nil {
			return resp.Status != ocsp.Good, nil
		}
		if checkOutcome(err) != checkRequestFailed {
			outcome = checkInvalidAnswer
		}
		reasons = append(reasons, fmt.Sprintf("'%s': %s", server, err))
	}
	return false, failCheck(outcome, fmt.Errorf("no usable OCSP answer: %s", strings.Join(reasons, "; ")))
}

// queryOCSP sends a request of up to 256 bytes with GET, encoded into the URL as RFC 6960 Appendix A.1 describes,
// and a longer one with POST.
func (rc *revocationChecker) queryOCSP(
	ctx context.Context,
	server string,
	ocspReq []byte,
	cert *x509.Certificate,
) (*ocsp.Response, error) {
	var body []byte
	var err error
	if len(ocspReq) <= 256 {
		body, err = rc.get(ctx, server+"/"+url.QueryEscape(base64.StdEncoding.EncodeToString(ocspReq)))
	} else {
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, server, bytes.NewReader(ocspReq))
		if err != nil {
			return nil, failCheck(checkRequestFailed, err)
		}
		req.Header.Set("Content-Type", "application/ocsp-request")
		body, err = rc.do(req)
	}
	if err != nil {
		return nil, err
	}
	// Without an issuer, the parser verifies the signature only against a certificate the answer embeds, and does not
	// require the issuer to have signed that certificate, which an intermediate CA's own certificate is not.
	// verifyOCSPResponse authenticates the signer.
	resp, err := ocsp.ParseResponseForCert(body, cert, nil)
	// Errors from outside the OCSP package can quote the answer, which anyone on the path can write.
	var parseErr ocsp.ParseError
	var respErr ocsp.ResponseError
	if err != nil && !errors.As(err, &parseErr) && !errors.As(err, &respErr) {
		return nil, errors.New("invalid OCSP response")
	}
	return resp, err
}

// findIssuer returns the certificate that signed cert: the first of issuers that did, or else the first one
// downloaded from the issuer URLs cert lists. When none is found, the outcome is a failed request only if every
// download request failed.
func (rc *revocationChecker) findIssuer(
	ctx context.Context,
	cert *x509.Certificate,
	issuers []*x509.Certificate,
) (*x509.Certificate, error) {
	for _, issuer := range issuers {
		if signedBy(cert, issuer) {
			return issuer, nil
		}
	}

	reasons := []string{"no certificate the source sent or TLS verification found signed it"}
	outcome := checkIssuerNotFound
	if !slices.ContainsFunc(cert.IssuingCertificateURL, isHTTPURL) {
		reasons = append(reasons, "the certificate lists no HTTP or HTTPS issuer URL")
	} else {
		outcome = checkRequestFailed
	}
	for _, issuerURL := range cert.IssuingCertificateURL {
		if !isHTTPURL(issuerURL) {
			continue
		}
		issuer, err := rc.downloadIssuer(ctx, cert, issuerURL)
		if err == nil {
			return issuer, nil
		}
		if checkOutcome(err) != checkRequestFailed {
			outcome = checkIssuerNotFound
		}
		reasons = append(reasons, err.Error())
	}
	return nil, failCheck(outcome, fmt.Errorf("issuer certificate not found: %s", strings.Join(reasons, "; ")))
}

// downloadIssuer returns the certificate at issuerURL if it signed cert.
func (rc *revocationChecker) downloadIssuer(
	ctx context.Context,
	cert *x509.Certificate,
	issuerURL string,
) (*x509.Certificate, error) {
	body, err := rc.get(ctx, issuerURL)
	if err != nil {
		return nil, err
	}
	// The parser error can quote the response, which anyone on the path can write, so it is replaced.
	issuer, err := parseCertificate(body)
	if err != nil {
		return nil, fmt.Errorf("'%s' does not serve a single DER or PEM certificate", issuerURL)
	}
	if !signedBy(cert, issuer) {
		return nil, fmt.Errorf("the certificate at '%s' did not sign it", issuerURL)
	}
	return issuer, nil
}

// signedBy reports whether issuer's key verifies cert's signature, which makes it the key of cert's issuer: CRLs and
// OCSP answers it verifies come from that issuer. Nothing else in a downloaded issuer certificate is authenticated,
// so only the signature is checked, as the CRL signature is. The key must also be in its canonical encoding: OCSP
// names the issuer by the hash of the encoding, and the certificate parser accepts an RSA key followed by other bytes.
func signedBy(cert, issuer *x509.Certificate) bool {
	spki, err := x509.MarshalPKIXPublicKey(issuer.PublicKey)
	return err == nil && bytes.Equal(spki, issuer.RawSubjectPublicKeyInfo) &&
		issuer.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature) == nil
}

// verifyCRL checks that crl is a current and complete CRL of cert's issuer that covers cert (RFC 5280 6.3.3).
func verifyCRL(crl *x509.RevocationList, cert, issuer *x509.Certificate, now time.Time) error {
	// The issuer certificate's own subject is not compared: only its key is authenticated, by cert's signature. The
	// names are not quoted: the CRL is not verified yet, and anyone on the path can write it.
	if !bytes.Equal(crl.RawIssuer, cert.RawIssuer) {
		return errors.New("issuer name differs from the certificate's issuer name (compared as encoded)")
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

// verifyOCSPResponse checks that the answer is signed with the issuer's key or by a responder the issuer authorized
// for OCSP signing (RFC 6960 4.2.2.2), identifies cert, and is current (RFC 6960 3.2).
func verifyOCSPResponse(resp *ocsp.Response, cert, issuer *x509.Certificate, now time.Time) error {
	signer := issuer
	// An embedded certificate with the issuer's key is the issuer's own, whoever issued it; any other is a delegated
	// responder's.
	if responder := resp.Certificate; responder != nil &&
		!bytes.Equal(responder.RawSubjectPublicKeyInfo, issuer.RawSubjectPublicKeyInfo) {
		if err := checkDelegatedResponder(responder, issuer, now); err != nil {
			return err
		}
		signer = responder
	}
	if err := resp.CheckSignatureFrom(signer); err != nil {
		return fmt.Errorf("bad OCSP signature: %w", err)
	}
	if err := checkOCSPCertID(resp, cert, issuer); err != nil {
		return err
	}
	return checkUpdateTimes(resp.ThisUpdate, resp.NextUpdate, now)
}

// checkDelegatedResponder checks that issuer certified responder for OCSP signing and that it is valid at now.
func checkDelegatedResponder(responder, issuer *x509.Certificate, now time.Time) error {
	if issuer.CheckSignature(responder.SignatureAlgorithm, responder.RawTBSCertificate, responder.Signature) != nil {
		return errors.New("responder certificate is not signed by the issuer")
	}
	if !slices.Contains(responder.ExtKeyUsage, x509.ExtKeyUsageOCSPSigning) {
		return errors.New("responder certificate is not authorized for OCSP signing")
	}
	// A subordinate CA that carries the OCSP-signing usage could otherwise answer for its issuer's certificates.
	if responder.IsCA {
		return errors.New("responder certificate is a CA certificate")
	}
	if now.Before(responder.NotBefore) || now.After(responder.NotAfter) {
		return errors.New("responder certificate is outside its validity period")
	}
	return nil
}

// checkOCSPCertID checks that the single response ocsp.ParseResponseForCert chose, by serial number alone, also names
// cert's issuer: by the hash of cert's issuer field and of the issuer's key (RFC 6960 4.1.1). Otherwise a responder
// key that serves two CAs could vouch for one CA's certificate with an answer about the other's.
func checkOCSPCertID(resp *ocsp.Response, cert, issuer *x509.Certificate) error {
	// ResponseData and SingleResponse (RFC 6960 4.2.1) up to the fields needed; asn1.Unmarshal ignores the rest.
	var data struct {
		Version     int `asn1:"optional,explicit,default:0,tag:0"`
		ResponderID asn1.RawValue
		ProducedAt  time.Time `asn1:"generalized"`
		Responses   []struct {
			CertID struct {
				HashAlgorithm  pkix.AlgorithmIdentifier
				IssuerNameHash []byte
				IssuerKeyHash  []byte
				SerialNumber   *big.Int
			}
		}
	}
	var spki struct {
		Algorithm pkix.AlgorithmIdentifier
		PublicKey asn1.BitString
	}
	if _, err := asn1.Unmarshal(resp.TBSResponseData, &data); err != nil {
		return err
	}
	if _, err := asn1.Unmarshal(issuer.RawSubjectPublicKeyInfo, &spki); err != nil {
		return err
	}
	if !resp.IssuerHash.Available() {
		return errors.New("unsupported certificate ID hash")
	}
	hash := func(b []byte) []byte {
		h := resp.IssuerHash.New()
		h.Write(b)
		return h.Sum(nil)
	}

	for _, single := range data.Responses {
		id := single.CertID
		if id.SerialNumber.Cmp(cert.SerialNumber) != 0 {
			continue
		}
		if !bytes.Equal(id.IssuerNameHash, hash(cert.RawIssuer)) ||
			!bytes.Equal(id.IssuerKeyHash, hash(spki.PublicKey.RightAlign())) {
			return errors.New("answer is about a certificate of another issuer")
		}
		return nil
	}
	return errors.New("answer is not about the certificate")
}

// checkUpdateTimes checks that now falls between a CRL's or an OCSP answer's this-update and next-update times, within
// revocationClockSkew. Without a next update, newer information is available all the time (RFC 6960 4.2.2.1), so
// the answer is current only at its this-update time.
func checkUpdateTimes(thisUpdate, nextUpdate, now time.Time) error {
	if nextUpdate.IsZero() {
		nextUpdate = thisUpdate
	}
	if thisUpdate.After(now.Add(revocationClockSkew)) {
		return fmt.Errorf("not valid until %s", thisUpdate)
	}
	if now.After(nextUpdate.Add(revocationClockSkew)) {
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
		return nil, failCheck(checkRequestFailed, err)
	}
	return rc.do(req)
}

// do returns the body of a 200 OK response. Its errors name the URL and are failed requests, except a body over the
// client's size limit, which is an answer that cannot be used.
func (rc *revocationChecker) do(req *http.Request) ([]byte, error) {
	var body []byte
	err := web.DoHTTP(rc.client).Request(req, func(r io.Reader) (err error) {
		body, err = io.ReadAll(r)
		return err
	})
	switch {
	case errors.Is(err, web.ErrResponseTooLarge):
		return nil, failCheck(checkInvalidAnswer, err)
	case err != nil:
		return nil, failCheck(checkRequestFailed, err)
	}
	return body, nil
}

// isHTTPURL reports a URL the checker fetches. CRL, OCSP and issuer URLs of other schemes, such as the LDAP URLs some
// CAs list, are not used.
func isHTTPURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
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
