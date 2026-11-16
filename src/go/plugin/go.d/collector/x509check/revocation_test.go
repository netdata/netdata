// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/cryptobyte"
	cryptobyte_asn1 "golang.org/x/crypto/cryptobyte/asn1"
	"golang.org/x/crypto/ocsp"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func TestRevocationChecker_Status(t *testing.T) {
	tests := map[string]struct {
		endpoints testPKIEndpoints
		// chain is what the source presents; the server certificate alone by default.
		chain func(*testPKI) []*x509.Certificate
		at    func(*testPKI) time.Time
		// want is the status; wantErr is part of the reason and wantOutcome the outcome when it cannot be determined.
		want        bool
		wantErr     string
		wantOutcome string
	}{
		"no CRL or OCSP URL": {
			wantErr:     "lists no HTTP or HTTPS CRL or OCSP URL",
			wantOutcome: checkNoRevocationInfo,
		},
		"CRL lists the certificate": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveCRL(true)},
			},
			want: true,
		},
		"CRL does not list the certificate": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveCRL(false)},
			},
		},
		"PEM CRL and issuer": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuerPEM},
				crls:    []testPKIHandler{serveCRLPEM(true)},
			},
			want: true,
		},
		"CRL with an unreachable issuer is not used": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{notFound},
				crls:    []testPKIHandler{unexpected},
			},
			wantErr:     "TLS verification found signed it; GET",
			wantOutcome: checkRequestFailed,
		},
		"CRL without an issuer URL is not used": {
			endpoints: testPKIEndpoints{
				crls: []testPKIHandler{unexpected},
			},
			wantErr:     "the certificate lists no HTTP or HTTPS issuer URL",
			wantOutcome: checkIssuerNotFound,
		},
		"issuer that did not sign the certificate": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveRogueIssuer},
				crls:    []testPKIHandler{unexpected},
			},
			wantErr:     "did not sign it",
			wantOutcome: checkIssuerNotFound,
		},
		"issuer that did not sign the certificate, for OCSP": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveRogueIssuer},
				ocsps:   []testPKIHandler{unexpected},
			},
			wantErr:     "did not sign it",
			wantOutcome: checkIssuerNotFound,
		},
		"issuer URLs unreachable or serving another certificate": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{notFound, serveRogueIssuer},
				crls:    []testPKIHandler{unexpected},
			},
			wantErr:     "did not sign it",
			wantOutcome: checkIssuerNotFound,
		},
		"issuer URLs serving another certificate or unreachable": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveRogueIssuer, notFound},
				crls:    []testPKIHandler{unexpected},
			},
			wantErr:     "did not sign it",
			wantOutcome: checkIssuerNotFound,
		},
		"issuer URL that is not HTTP": {
			endpoints: testPKIEndpoints{
				crls: []testPKIHandler{unexpected},
			},
			chain: func(p *testPKI) []*x509.Certificate {
				leaf := p.issueLeaf(p.t, p.leaf.SerialNumber, p.ca, p.caKey, func(template *x509.Certificate) {
					template.IssuingCertificateURL = []string{"ldap:///CN=CA,CN=AIA,CN=Public%20Key%20Services?cACertificate"}
				})
				return []*x509.Certificate{leaf}
			},
			wantErr:     "lists no HTTP or HTTPS issuer URL",
			wantOutcome: checkIssuerNotFound,
		},
		"CRL and OCSP URLs that are not HTTP": {
			chain: func(p *testPKI) []*x509.Certificate {
				leaf := p.issueLeaf(p.t, p.leaf.SerialNumber, p.ca, p.caKey, func(template *x509.Certificate) {
					template.CRLDistributionPoints = []string{"ldaps://ldap.example.org/CN=CA?certificateRevocationList"}
					template.OCSPServer = []string{"ftp://ocsp.example.org"}
				})
				return []*x509.Certificate{leaf}
			},
			wantErr:     "lists no HTTP or HTTPS CRL or OCSP URL",
			wantOutcome: checkNoRevocationInfo,
		},
		"issuer URLs unreachable": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{notFound, notFound},
				ocsps:   []testPKIHandler{unexpected},
			},
			wantErr:     "issuer certificate not found: no certificate the source sent",
			wantOutcome: checkRequestFailed,
		},
		"issuer URL after one that did not sign the certificate": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveRogueIssuer, serveIssuer},
				crls:    []testPKIHandler{serveCRL(true)},
			},
			want: true,
		},
		"issuer from the presented chain": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{unexpected},
				crls:    []testPKIHandler{serveCRL(true)},
			},
			chain: func(p *testPKI) []*x509.Certificate { return []*x509.Certificate{p.leaf, p.rogue, p.ca} },
			want:  true,
		},
		"SHA-1 signed certificate": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveCRL(true)},
			},
			chain: func(p *testPKI) []*x509.Certificate {
				leaf := p.issueLeaf(p.t, p.leaf.SerialNumber, p.ca, p.caKey, func(template *x509.Certificate) {
					template.SignatureAlgorithm = x509.ECDSAWithSHA1
				})
				return []*x509.Certificate{leaf}
			},
			want: true,
		},
		"presented chain without the issuer": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveCRL(true)},
			},
			chain: func(p *testPKI) []*x509.Certificate { return []*x509.Certificate{p.leaf, p.rogue} },
			want:  true,
		},
		"CRL signed by another CA": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveRogueCRL},
			},
			wantErr:     "verification failure",
			wantOutcome: checkInvalidAnswer,
		},
		"CRL from another issuer name with the issuer's key": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveRenamedCRL},
			},
			wantErr:     "differs from the certificate's issuer name",
			wantOutcome: checkInvalidAnswer,
		},
		"CRL past its next update": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveCRL(false)},
			},
			at:          func(p *testPKI) time.Time { return p.nextUpdate().Add(revocationClockSkew + time.Second) },
			wantErr:     "outdated since",
			wantOutcome: checkInvalidAnswer,
		},
		"CRL past its next update within the clock skew": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveCRL(false)},
			},
			at: func(p *testPKI) time.Time { return p.nextUpdate().Add(revocationClockSkew) },
		},
		"CRL before its this update": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveCRL(false)},
			},
			at:          func(p *testPKI) time.Time { return p.thisUpdate().Add(-revocationClockSkew - time.Second) },
			wantErr:     "not valid until",
			wantOutcome: checkInvalidAnswer,
		},
		"CRL before its this update within the clock skew": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveCRL(false)},
			},
			at: func(p *testPKI) time.Time { return p.thisUpdate().Add(-revocationClockSkew) },
		},
		"CRL partition for the certificate": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls: []testPKIHandler{serveCRLWith(func(p *testPKI, rl *x509.RevocationList) {
					rl.ExtraExtensions = []pkix.Extension{idpExtension([]string{p.crlURLs[0]}, idpOnlyUserCerts)}
				})},
			},
			want: true,
		},
		"CRL partition for other certificates": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls: []testPKIHandler{serveCRLWith(func(p *testPKI, rl *x509.RevocationList) {
					rl.ExtraExtensions = []pkix.Extension{idpExtension([]string{p.crlURLs[0] + "-other"})}
				})},
			},
			wantErr:     "names none of the certificate's CRL URLs",
			wantOutcome: checkInvalidAnswer,
		},
		"CRL of CA certificates": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls: []testPKIHandler{serveCRLWith(func(_ *testPKI, rl *x509.RevocationList) {
					rl.ExtraExtensions = []pkix.Extension{idpExtension(nil, idpOnlyCACerts)}
				})},
			},
			wantErr:     "covers only CA certificates",
			wantOutcome: checkInvalidAnswer,
		},
		"delta CRL": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls: []testPKIHandler{serveCRLWith(func(_ *testPKI, rl *x509.RevocationList) {
					rl.ExtraExtensions = []pkix.Extension{{
						Id:       asn1.ObjectIdentifier{2, 5, 29, 27}, // delta CRL indicator
						Critical: true,
						Value:    []byte{0x02, 0x01, 0x01},
					}}
				})},
			},
			wantErr:     "unsupported critical extension 2.5.29.27",
			wantOutcome: checkInvalidAnswer,
		},
		"CRL with a non-critical extension it does not process": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls: []testPKIHandler{serveCRLWith(func(_ *testPKI, rl *x509.RevocationList) {
					rl.ExtraExtensions = []pkix.Extension{{
						Id:    asn1.ObjectIdentifier{2, 5, 29, 46}, // freshest CRL
						Value: []byte{0x30, 0x00},
					}}
				})},
			},
			want: true,
		},
		"CRL entry with a critical extension": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls: []testPKIHandler{serveCRLWith(func(_ *testPKI, rl *x509.RevocationList) {
					rl.RevokedCertificateEntries[0].ExtraExtensions = []pkix.Extension{{
						Id:       asn1.ObjectIdentifier{2, 5, 29, 29}, // certificate issuer, of an indirect CRL
						Critical: true,
						Value:    []byte{0x30, 0x00},
					}}
				})},
			},
			wantErr:     "unsupported critical entry extension 2.5.29.29",
			wantOutcome: checkInvalidAnswer,
		},
		"CRL entry with a non-critical extension": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls: []testPKIHandler{serveCRLWith(func(_ *testPKI, rl *x509.RevocationList) {
					rl.RevokedCertificateEntries[0].ReasonCode = 1 // keyCompromise
				})},
			},
			want: true,
		},
		"CRL unreachable": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{notFound},
			},
			wantErr:     "returned HTTP status code: 404",
			wantOutcome: checkRequestFailed,
		},
		"CRL is not a CRL": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveIssuer},
			},
			wantErr:     "parse CRL",
			wantOutcome: checkInvalidAnswer,
		},
		"LDAP CRL is skipped": {
			endpoints: testPKIEndpoints{
				ldapCRL: true,
			},
			wantErr:     "lists no HTTP or HTTPS CRL or OCSP URL",
			wantOutcome: checkNoRevocationInfo,
		},
		"LDAP CRL is skipped for OCSP": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveOCSP(ocsp.Revoked)},
				ldapCRL: true,
			},
			want: true,
		},
		"later CRL lists the certificate": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveCRL(false), serveCRL(true)},
				ldapCRL: true,
			},
			want: true,
		},
		"issuer without the CRL-signing key usage": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuerWithoutCRLSign},
				crls:    []testPKIHandler{serveCRL(true)},
			},
			want: true,
		},
		"revoked by CRL without asking OCSP": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveCRL(true)},
				ocsps:   []testPKIHandler{unexpected},
			},
			want: true,
		},
		"OCSP good": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveOCSP(ocsp.Good)},
			},
		},
		"OCSP revoked": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveOCSP(ocsp.Revoked)},
			},
			want: true,
		},
		"OCSP unknown is reported as revoked": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveOCSP(ocsp.Unknown)},
			},
			want: true,
		},
		"OCSP after a CRL that does not list the certificate": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveCRL(false)},
				ocsps:   []testPKIHandler{serveOCSP(ocsp.Revoked)},
			},
			want: true,
		},
		"failing OCSP responder is skipped": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{notFound, serveIssuer},
				ocsps:   []testPKIHandler{notFound, serveOCSP(ocsp.Good)},
			},
		},
		"OCSP responders unreachable": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{notFound, notFound},
			},
			// The prefix names the troubleshooting entry.
			wantErr:     "no usable OCSP answer: '",
			wantOutcome: checkRequestFailed,
		},
		"OCSP responders unreachable or with unusable answers": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{notFound, serveRogueOCSP},
			},
			wantErr:     "bad OCSP signature",
			wantOutcome: checkInvalidAnswer,
		},
		"OCSP responders with unusable answers or unreachable": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveRogueOCSP, notFound},
			},
			wantErr:     "bad OCSP signature",
			wantOutcome: checkInvalidAnswer,
		},
		"OCSP answer signed by another CA": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveRogueOCSP},
			},
			wantErr:     "bad OCSP signature",
			wantOutcome: checkInvalidAnswer,
		},
		"OCSP answer for another certificate": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveOCSPForAnotherCertificate},
			},
			wantErr:     "no response matching the supplied certificate",
			wantOutcome: checkInvalidAnswer,
		},
		"OCSP answer past its next update": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveOCSP(ocsp.Good)},
			},
			at:          func(p *testPKI) time.Time { return p.nextUpdate().Add(revocationClockSkew + time.Second) },
			wantErr:     "outdated since",
			wantOutcome: checkInvalidAnswer,
		},
		"OCSP answer past its next update within the clock skew": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveOCSP(ocsp.Good)},
			},
			at: func(p *testPKI) time.Time { return p.nextUpdate().Add(revocationClockSkew) },
		},
		"OCSP answer before its this update": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveOCSP(ocsp.Good)},
			},
			at:          func(p *testPKI) time.Time { return p.thisUpdate().Add(-revocationClockSkew - time.Second) },
			wantErr:     "not valid until",
			wantOutcome: checkInvalidAnswer,
		},
		"OCSP answer before its this update within the clock skew": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveOCSP(ocsp.Good)},
			},
			at: func(p *testPKI) time.Time { return p.thisUpdate().Add(-revocationClockSkew) },
		},
		"OCSP answer without a next update within the clock skew of its this update": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps: []testPKIHandler{serveOCSPWith(func(_ *testPKI, answer *ocsp.Response) {
					answer.NextUpdate = time.Time{}
				})},
			},
			at: func(p *testPKI) time.Time { return p.thisUpdate().Add(revocationClockSkew) },
		},
		"OCSP answer without a next update past the clock skew of its this update": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps: []testPKIHandler{serveOCSPWith(func(_ *testPKI, answer *ocsp.Response) {
					answer.NextUpdate = time.Time{}
				})},
			},
			at:          func(p *testPKI) time.Time { return p.thisUpdate().Add(revocationClockSkew + time.Second) },
			wantErr:     "outdated since",
			wantOutcome: checkInvalidAnswer,
		},
		"OCSP answer about a certificate of an issuer with another name": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveOCSPAbout(func(p *testPKI) *x509.Certificate { return p.caRenamed })},
			},
			wantErr:     "about a certificate of another issuer",
			wantOutcome: checkInvalidAnswer,
		},
		"OCSP answer about a certificate of an issuer with another key": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveOCSPAbout(func(p *testPKI) *x509.Certificate { return p.rogue })},
			},
			wantErr:     "about a certificate of another issuer",
			wantOutcome: checkInvalidAnswer,
		},
		"OCSP request names the certificate's issuer, not the issuer certificate's subject": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{unexpected},
				ocsps:   []testPKIHandler{serveOCSP(ocsp.Good)},
			},
			chain: func(p *testPKI) []*x509.Certificate { return []*x509.Certificate{p.leaf, p.caRenamed} },
		},
		"stale OCSP answer is skipped": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps: []testPKIHandler{
					serveOCSPWith(func(p *testPKI, answer *ocsp.Response) {
						answer.ThisUpdate = p.now.Add(-2 * time.Hour)
						answer.NextUpdate = p.now.Add(-time.Hour)
					}),
					serveOCSP(ocsp.Revoked),
				},
			},
			want: true,
		},
		"OCSP answer with the issuer's certificate": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps: []testPKIHandler{serveOCSPWith(func(p *testPKI, answer *ocsp.Response) {
					answer.Certificate = p.ca
				})},
			},
		},
		// An intermediate CA that signs its answers itself sends its own certificate, which its root issued, as
		// OpenSSL's responder does by default.
		"OCSP answer with the issuer's certificate from another CA": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps: []testPKIHandler{serveOCSPWith(func(p *testPKI, answer *ocsp.Response) {
					answer.Certificate = p.caIssuedByRogue
				})},
			},
		},
		"OCSP answer from an authorized responder": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveDelegatedOCSP(func(*testPKI, *x509.Certificate) {})},
			},
		},
		"OCSP answer from a responder not authorized for OCSP signing": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps: []testPKIHandler{serveDelegatedOCSP(func(_ *testPKI, responder *x509.Certificate) {
					responder.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
				})},
			},
			wantErr:     "not authorized for OCSP signing",
			wantOutcome: checkInvalidAnswer,
		},
		"OCSP answer from a responder another CA certified": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps: []testPKIHandler{serveDelegatedOCSPFrom(
					func(p *testPKI) (*x509.Certificate, *ecdsa.PrivateKey) { return p.rogue, p.rogueKey },
					func(*testPKI, *x509.Certificate) {},
				)},
			},
			wantErr:     "responder certificate is not signed by the issuer",
			wantOutcome: checkInvalidAnswer,
		},
		"OCSP answer from an expired responder": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps: []testPKIHandler{serveDelegatedOCSP(func(p *testPKI, responder *x509.Certificate) {
					responder.NotAfter = p.now.Add(-time.Second)
				})},
			},
			wantErr:     "outside its validity period",
			wantOutcome: checkInvalidAnswer,
		},
		"OCSP answer from a responder not yet valid": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps: []testPKIHandler{serveDelegatedOCSP(func(p *testPKI, responder *x509.Certificate) {
					responder.NotBefore = p.now.Add(time.Second)
				})},
			},
			wantErr:     "outside its validity period",
			wantOutcome: checkInvalidAnswer,
		},
		"OCSP answer from a CA certificate authorized for OCSP signing": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps: []testPKIHandler{serveDelegatedOCSP(func(_ *testPKI, responder *x509.Certificate) {
					responder.IsCA = true
					responder.BasicConstraintsValid = true
					responder.KeyUsage |= x509.KeyUsageCertSign
				})},
			},
			wantErr:     "is a CA certificate",
			wantOutcome: checkInvalidAnswer,
		},
		"OCSP without an issuer URL": {
			endpoints: testPKIEndpoints{
				ocsps: []testPKIHandler{unexpected},
			},
			wantErr:     "the certificate lists no HTTP or HTTPS issuer URL",
			wantOutcome: checkIssuerNotFound,
		},
		"OCSP with an unreachable issuer": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{notFound},
				ocsps:   []testPKIHandler{unexpected},
			},
			// The prefix names the troubleshooting entry.
			wantErr:     "issuer certificate not found: no certificate the source sent",
			wantOutcome: checkRequestFailed,
		},
		"expired certificate": {
			endpoints: testPKIEndpoints{
				crls: []testPKIHandler{unexpected},
			},
			at:          func(p *testPKI) time.Time { return p.leaf.NotAfter.Add(time.Second) },
			wantErr:     "certificate expired at",
			wantOutcome: checkOutsideValidity,
		},
		"certificate not yet valid": {
			endpoints: testPKIEndpoints{
				crls: []testPKIHandler{unexpected},
			},
			at:          func(p *testPKI) time.Time { return p.leaf.NotBefore.Add(-time.Second) },
			wantErr:     "certificate is not valid until",
			wantOutcome: checkOutsideValidity,
		},
		// The certificate lists no URL, so passing the validity check, which comes first, ends the check there.
		"certificate at its last valid instant": {
			at:          func(p *testPKI) time.Time { return p.leaf.NotAfter },
			wantErr:     "lists no HTTP or HTTPS CRL or OCSP URL",
			wantOutcome: checkNoRevocationInfo,
		},
		"certificate at its first valid instant": {
			at:          func(p *testPKI) time.Time { return p.leaf.NotBefore },
			wantErr:     "lists no HTTP or HTTPS CRL or OCSP URL",
			wantOutcome: checkNoRevocationInfo,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pki := newTestPKI(t, test.endpoints)
			chain := []*x509.Certificate{pki.leaf}
			if test.chain != nil {
				chain = test.chain(pki)
			}
			at := pki.now
			if test.at != nil {
				at = test.at(pki)
			}

			revoked, err := newRevocationChecker().status(t.Context(), chain[0], chain[1:], at)

			if test.wantErr != "" {
				assert.ErrorContains(t, err, test.wantErr)
				assert.Equal(t, test.wantOutcome, checkOutcome(err))
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.want, revoked)
			}
			assert.LessOrEqual(t, pki.hits["issuer"].Load(), int64(len(test.endpoints.issuers)),
				"a check requests each issuer URL at most once")
		})
	}
}

func TestRevocationChecker_AnswerOverTheSizeLimit(t *testing.T) {
	pki := newTestPKI(t, testPKIEndpoints{
		crls: []testPKIHandler{serveCRL(false)},
	})
	rc := newRevocationChecker()
	// The CRL is larger than this limit. The issuer comes with the chain, so the CRL is the only download.
	rc.client = web.NewHTTPClientWithTransport(web.ClientConfig{
		Timeout:           confopt.Duration(revocationRequestTimeout),
		ResponseBodyLimit: 64,
	}, nil)

	_, err := rc.status(t.Context(), pki.leaf, []*x509.Certificate{pki.ca}, pki.now)

	assert.ErrorIs(t, err, web.ErrResponseTooLarge)
	assert.Equal(t, checkInvalidAnswer, checkOutcome(err))
}

func TestRevocationChecker_CachesCRLVerdictUntilNextUpdate(t *testing.T) {
	pki := newTestPKI(t, testPKIEndpoints{
		issuers: []testPKIHandler{serveIssuer},
		crls:    []testPKIHandler{serveCRL(true)},
	})
	// A second certificate behind the same source, as with round-robin DNS.
	other := pki.issueLeaf(t, big.NewInt(2), pki.ca, pki.caKey)
	// The same serial from a CA with another name is another certificate.
	otherCA, otherCAKey := newTestCA(t, pki.now, "Other CA")
	otherIssuer := pki.issueLeaf(t, pki.leaf.SerialNumber, otherCA, otherCAKey)
	rc := newRevocationChecker()

	type downloads struct {
		crl, issuer int64
	}
	for _, step := range []struct {
		name          string
		chain         []*x509.Certificate
		at            time.Time
		wantRevoked   bool
		wantErr       bool
		wantDownloads downloads
	}{
		{
			name:        "first check downloads",
			chain:       []*x509.Certificate{pki.leaf},
			at:          pki.now,
			wantRevoked: true,
			wantDownloads: downloads{
				crl:    1,
				issuer: 1,
			},
		},
		{
			name:        "cached verdict before the next update",
			chain:       []*x509.Certificate{pki.leaf},
			at:          pki.nextUpdate().Add(-time.Second),
			wantRevoked: true,
			wantDownloads: downloads{
				crl:    1,
				issuer: 1,
			},
		},
		{
			name:        "another serial downloads",
			chain:       []*x509.Certificate{other},
			at:          pki.now,
			wantRevoked: false,
			wantDownloads: downloads{
				crl:    2,
				issuer: 2,
			},
		},
		{
			name:        "first serial is still cached",
			chain:       []*x509.Certificate{pki.leaf},
			at:          pki.now,
			wantRevoked: true,
			wantDownloads: downloads{
				crl:    2,
				issuer: 2,
			},
		},
		{
			name:        "another serial is cached too",
			chain:       []*x509.Certificate{other},
			at:          pki.now,
			wantRevoked: false,
			wantDownloads: downloads{
				crl:    2,
				issuer: 2,
			},
		},
		{
			// The CRL at the URL is the first CA's, so it does not apply.
			name:    "the first serial from another issuer is not served from the cache",
			chain:   []*x509.Certificate{otherIssuer, otherCA},
			at:      pki.now,
			wantErr: true,
			wantDownloads: downloads{
				crl:    3,
				issuer: 2,
			},
		},
		{
			name:    "a rejected CRL is not cached",
			chain:   []*x509.Certificate{otherIssuer, otherCA},
			at:      pki.now,
			wantErr: true,
			wantDownloads: downloads{
				crl:    4,
				issuer: 2,
			},
		},
		{
			name:        "next update downloads again",
			chain:       []*x509.Certificate{pki.leaf},
			at:          pki.nextUpdate(),
			wantRevoked: true,
			wantDownloads: downloads{
				crl:    5,
				issuer: 3,
			},
		},
	} {
		revoked, err := rc.status(t.Context(), step.chain[0], step.chain[1:], step.at)

		if step.wantErr {
			assert.ErrorContains(t, err, "differs from the certificate's issuer name", step.name)
		} else {
			require.NoError(t, err, step.name)
			assert.Equal(t, step.wantRevoked, revoked, step.name)
		}
		assert.Equal(t, step.wantDownloads, downloads{
			crl:    pki.hits["crl"].Load(),
			issuer: pki.hits["issuer"].Load(),
		}, step.name)
	}
}

func TestRevocationChecker_ReasonsDoNotQuoteServedData(t *testing.T) {
	// A certificate that the parser rejects with an error that quotes its URI, which anyone on the path can write.
	quoted := strings.Repeat("q", 1000)
	unparsable := func(p *testPKI) []byte {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(p.t, err)
		template := &x509.Certificate{
			SerialNumber: big.NewInt(3),
			NotBefore:    p.now.Add(-time.Hour),
			NotAfter:     p.now.Add(time.Hour),
			URIs: []*url.URL{{
				Scheme: "https",
				Host:   quoted + ".",
			}},
		}
		der, err := x509.CreateCertificate(rand.Reader, template, p.ca, &key.PublicKey, p.caKey)
		require.NoError(p.t, err)
		_, err = x509.ParseCertificate(der)
		require.ErrorContains(p.t, err, quoted)
		return der
	}

	tests := map[string]struct {
		endpoints testPKIEndpoints
		wantErr   string
	}{
		"issuer URL": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{func(p *testPKI, w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write(unparsable(p))
				}},
				crls: []testPKIHandler{unexpected},
			},
			wantErr: "does not serve a single DER or PEM certificate",
		},
		"certificate in an OCSP answer": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps: []testPKIHandler{serveOCSPWith(func(p *testPKI, answer *ocsp.Response) {
					answer.Certificate = &x509.Certificate{
						Raw: unparsable(p),
					}
				})},
			},
			wantErr: "invalid OCSP response",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pki := newTestPKI(t, test.endpoints)

			_, err := newRevocationChecker().status(t.Context(), pki.leaf, nil, pki.now)

			require.ErrorContains(t, err, test.wantErr)
			assert.NotContains(t, err.Error(), quoted)
		})
	}
}

func TestRevocationChecker_RequestsAreBounded(t *testing.T) {
	pki := newTestPKI(t, testPKIEndpoints{
		crls: []testPKIHandler{stall},
	})
	rc := newRevocationChecker()
	require.Positive(t, rc.client.Timeout)
	rc.client.Timeout = 50 * time.Millisecond

	_, err := rc.status(t.Context(), pki.leaf, []*x509.Certificate{pki.ca}, pki.now)

	// The test context has no deadline, so only the client timeout can end the request this way.
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRevocationChecker_OCSPRequestMethod(t *testing.T) {
	tests := map[string]struct {
		size       int
		wantMethod string
	}{
		"256-byte request uses GET": {
			size:       256,
			wantMethod: http.MethodGet,
		},
		"257-byte request uses POST": {
			size:       257,
			wantMethod: http.MethodPost,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ocspReq := bytes.Repeat([]byte{0x30}, test.size)
			pki := newTestPKI(t, testPKIEndpoints{
				ocsps: []testPKIHandler{func(p *testPKI, w http.ResponseWriter, r *http.Request) {
					der, err := p.ocspRequestBytes(r)
					assert.NoError(p.t, err)
					assert.Equal(p.t, test.wantMethod, r.Method)
					assert.Equal(p.t, ocspReq, der)
					p.writeOCSP(w, p.ca, p.caKey, p.ocspAnswer(ocsp.Good))
				}},
			})

			resp, err := newRevocationChecker().queryOCSP(t.Context(), pki.leaf.OCSPServer[0], ocspReq, pki.leaf)

			require.NoError(t, err)
			assert.Equal(t, ocsp.Good, resp.Status)
		})
	}
}

func TestSignedBy(t *testing.T) {
	now := time.Now()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "RSA CA",
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err = x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(2),
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
	}, ca, &leafKey.PublicKey, caKey)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	// The CA's key followed by other bytes, which the certificate parser accepts as the same key; OCSP would name the
	// issuer by the hash of these bytes.
	var spki struct {
		Algorithm pkix.AlgorithmIdentifier
		PublicKey asn1.BitString
	}
	_, err = asn1.Unmarshal(ca.RawSubjectPublicKeyInfo, &spki)
	require.NoError(t, err)
	spki.PublicKey.Bytes = append(spki.PublicKey.Bytes, 0x00)
	spki.PublicKey.BitLength += 8
	padded, err := asn1.Marshal(spki)
	require.NoError(t, err)
	paddedKey, err := x509.ParsePKIXPublicKey(padded)
	require.NoError(t, err)
	require.Equal(t, ca.PublicKey, paddedKey)
	forged := *ca
	forged.RawSubjectPublicKeyInfo = padded

	tests := map[string]struct {
		issuer *x509.Certificate
		want   bool
	}{
		"issuer": {
			issuer: ca,
			want:   true,
		},
		"issuer key encoded with trailing bytes": {
			issuer: &forged,
			want:   false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, signedBy(leaf, test.issuer))
		})
	}
}

func TestCheckIssuingDistributionPoint(t *testing.T) {
	const crlURL = "http://ye2.c.lencr.org/51.crl"
	mustHex := func(s string) []byte {
		b, err := hex.DecodeString(s)
		require.NoError(t, err)
		return b
	}
	// The extension of a Let's Encrypt CRL partition: crlURL as its full name, and only end-entity certificates.
	letsEncrypt := mustHex("3026a021a01f861d687474703a2f2f7965322e632e6c656e63722e6f72672f35312e63726c8101ff")

	tests := map[string]struct {
		der     []byte
		ca      bool
		wantErr bool
	}{
		"Let's Encrypt partition": {
			der: letsEncrypt,
		},
		"Let's Encrypt partition for a CA certificate": {
			der:     letsEncrypt,
			ca:      true,
			wantErr: true,
		},
		"another partition": {
			der:     idpDER([]string{"http://ye2.c.lencr.org/52.crl"}),
			wantErr: true,
		},
		"one of several names": {
			der: idpDER([]string{"ldap://ldap.example.com/cn=CA", crlURL}),
		},
		"no distribution point name": {
			der: idpDER(nil),
		},
		"name relative to the CRL issuer": {
			// distributionPoint nameRelativeToCRLIssuer CN=CA
			der:     mustHex("300fa00da10b300906035504030c024341"),
			wantErr: true,
		},
		"URL as a DNS name": {
			// distributionPoint fullName dNSName crlURL
			der:     mustHex("3023a021a01f821d687474703a2f2f7965322e632e6c656e63722e6f72672f35312e63726c"),
			wantErr: true,
		},
		"full name without a URI": {
			// distributionPoint fullName directoryName CN=CA
			der:     mustHex("3015a013a011a40f300d310b300906035504030c024341"),
			wantErr: true,
		},
		"only CA certificates": {
			der:     idpDER(nil, idpOnlyCACerts),
			wantErr: true,
		},
		"only CA certificates for a CA certificate": {
			der: idpDER(nil, idpOnlyCACerts),
			ca:  true,
		},
		"only some reasons": {
			der:     idpDER(nil, idpOnlySomeReasons),
			wantErr: true,
		},
		"indirect CRL": {
			der:     idpDER(nil, idpIndirectCRL),
			wantErr: true,
		},
		"only attribute certificates": {
			der:     idpDER(nil, idpOnlyAttributeCerts),
			wantErr: true,
		},
		"flags encoded as false": {
			// onlyContainsUserCerts and onlyContainsCACerts FALSE
			der: mustHex("3006810100820100"),
		},
		"not a sequence": {
			der:     mustHex("0400"),
			wantErr: true,
		},
		"truncated": {
			der:     mustHex("30058101ff"),
			wantErr: true,
		},
		"malformed full name": {
			// a URI that claims 5 octets and has none
			der:     mustHex("3006a004a0028605"),
			wantErr: true,
		},
		"flag of two octets": {
			der:     mustHex("30048102ffff"),
			wantErr: true,
		},
		"flag neither true nor false": {
			der:     mustHex("3003810101"),
			wantErr: true,
		},
		"unknown field": {
			der:     mustHex("3003860100"),
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cert := &x509.Certificate{
				BasicConstraintsValid: test.ca,
				IsCA:                  test.ca,
				CRLDistributionPoints: []string{crlURL},
			}

			err := checkIssuingDistributionPoint(test.der, cert)

			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Tags of the issuing distribution point flags (RFC 5280 5.2.5).
const (
	idpOnlyUserCerts = iota + 1
	idpOnlyCACerts
	idpOnlySomeReasons
	idpIndirectCRL
	idpOnlyAttributeCerts
)

// idpDER encodes an issuing distribution point with uris as its full name, when any, and the flags with tags set.
func idpDER(uris []string, tags ...int) []byte {
	b := cryptobyte.NewBuilder(nil)
	b.AddASN1(cryptobyte_asn1.SEQUENCE, func(b *cryptobyte.Builder) {
		if len(uris) > 0 {
			b.AddASN1(cryptobyte_asn1.Tag(0).Constructed().ContextSpecific(), func(b *cryptobyte.Builder) {
				b.AddASN1(cryptobyte_asn1.Tag(0).Constructed().ContextSpecific(), func(b *cryptobyte.Builder) {
					for _, uri := range uris {
						b.AddASN1(cryptobyte_asn1.Tag(6).ContextSpecific(), func(b *cryptobyte.Builder) {
							b.AddBytes([]byte(uri))
						})
					}
				})
			})
		}
		for _, tag := range tags {
			b.AddASN1(cryptobyte_asn1.Tag(tag).ContextSpecific(), func(b *cryptobyte.Builder) {
				if tag == idpOnlySomeReasons {
					b.AddBytes([]byte{0x06, 0x40}) // keyCompromise
				} else {
					b.AddUint8(0xff)
				}
			})
		}
	})
	return b.BytesOrPanic()
}

// idpExtension is a critical issuing distribution point CRL extension, as idpDER encodes it.
func idpExtension(uris []string, tags ...int) pkix.Extension {
	return pkix.Extension{
		Id:       oidExtensionIssuingDistributionPoint,
		Critical: true,
		Value:    idpDER(uris, tags...),
	}
}

// testPKI is a CA that serves its certificate, a CRL and OCSP answers over HTTP, and a server certificate it issued
// that lists those URLs. A rogue CA with the same name signs answers the real CA did not.
type testPKI struct {
	t     testing.TB
	now   time.Time
	ca    *x509.Certificate
	caKey *ecdsa.PrivateKey
	// caWithoutCRLSign holds the CA's key in a certificate that lacks the CRL-signing key usage.
	caWithoutCRLSign *x509.Certificate
	// caRenamed holds the CA's key in a certificate with another name.
	caRenamed *x509.Certificate
	// caIssuedByRogue holds the CA's key in a certificate another CA issued, as an intermediate CA's certificate is.
	caIssuedByRogue *x509.Certificate
	rogue           *x509.Certificate
	rogueKey        *ecdsa.PrivateKey
	leaf            *x509.Certificate

	crlURLs, ocspURLs, issuerURLs []string

	// hits counts requests per endpoint kind ("crl", "ocsp", "issuer").
	hits     map[string]*atomic.Int64
	inFlight atomic.Int64
}

type testPKIHandler func(p *testPKI, w http.ResponseWriter, r *http.Request)

// testPKIEndpoints lists the handlers behind each URL the server certificate lists, in order.
type testPKIEndpoints struct {
	issuers []testPKIHandler
	crls    []testPKIHandler
	ocsps   []testPKIHandler
	// ldapCRL lists an LDAP CRL distribution point before the HTTP ones.
	ldapCRL bool
}

func newTestPKI(t *testing.T, endpoints testPKIEndpoints) *testPKI {
	t.Helper()
	now := time.Now().Truncate(time.Second)
	ca, caKey := newTestCA(t, now, "Test CA")
	rogue, rogueKey := newTestCA(t, now, "Test CA")
	p := &testPKI{
		t:                t,
		now:              now,
		ca:               ca,
		caKey:            caKey,
		caWithoutCRLSign: selfSignCA(t, now, "Test CA", caKey, x509.KeyUsageCertSign),
		caRenamed:        selfSignCA(t, now, "Renamed CA", caKey, x509.KeyUsageCertSign|x509.KeyUsageCRLSign),
		caIssuedByRogue: signCA(
			t,
			now,
			"Test CA",
			&caKey.PublicKey,
			x509.KeyUsageCertSign|x509.KeyUsageCRLSign,
			rogue,
			rogueKey,
		),
		rogue:    rogue,
		rogueKey: rogueKey,
		hits: map[string]*atomic.Int64{
			"crl":    {},
			"ocsp":   {},
			"issuer": {},
		},
	}

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	serve := func(kind string, handlers []testPKIHandler) []string {
		var urls []string
		for i, handler := range handlers {
			endpoint := fmt.Sprintf("/%s/%d", kind, i)
			h := func(w http.ResponseWriter, r *http.Request) {
				p.hits[kind].Add(1)
				p.inFlight.Add(1)
				defer p.inFlight.Add(-1)
				handler(p, w, r)
			}
			mux.HandleFunc(endpoint, h)
			// An OCSP GET request appends the encoded request to the URL path.
			mux.HandleFunc(endpoint+"/", h)
			urls = append(urls, srv.URL+endpoint)
		}
		return urls
	}
	if endpoints.ldapCRL {
		// Upper case: URL schemes are case-insensitive.
		p.crlURLs = append(p.crlURLs, "LDAP://ldap.example.com/cn=Test%20CA?certificateRevocationList")
	}
	p.crlURLs = append(p.crlURLs, serve("crl", endpoints.crls)...)
	p.ocspURLs = serve("ocsp", endpoints.ocsps)
	p.issuerURLs = serve("issuer", endpoints.issuers)
	p.leaf = p.issueLeaf(t, big.NewInt(1), p.ca, p.caKey)
	return p
}

func newTestCA(t *testing.T, now time.Time, name string) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return selfSignCA(t, now, name, key, x509.KeyUsageCertSign|x509.KeyUsageCRLSign), key
}

func selfSignCA(
	t *testing.T,
	now time.Time,
	name string,
	key *ecdsa.PrivateKey,
	keyUsage x509.KeyUsage,
) *x509.Certificate {
	t.Helper()
	return signCA(t, now, name, &key.PublicKey, keyUsage, nil, key)
}

// signCA returns a CA certificate for pub that parent issued with parentKey, or that parentKey signs itself when
// parent is nil.
func signCA(
	t *testing.T,
	now time.Time,
	name string,
	pub *ecdsa.PublicKey,
	keyUsage x509.KeyUsage,
	parent *x509.Certificate,
	parentKey *ecdsa.PrivateKey,
) *x509.Certificate {
	t.Helper()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: name,
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(48 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              keyUsage,
	}
	if parent == nil {
		parent = template
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, pub, parentKey)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert
}

// issueLeaf issues a server certificate with serial, signed by issuer, that lists the PKI's URLs, after modify
// changes its template.
func (p *testPKI) issueLeaf(
	t testing.TB,
	serial *big.Int,
	issuer *x509.Certificate,
	issuerKey *ecdsa.PrivateKey,
	modify ...func(*x509.Certificate),
) *x509.Certificate {
	t.Helper()
	return p.issueServerCertificate(t, serial, issuer, issuerKey, modify...).Leaf
}

// issueServerCertificate is issueLeaf with the certificate's private key, for a TLS server.
func (p *testPKI) issueServerCertificate(
	t testing.TB,
	serial *big.Int,
	issuer *x509.Certificate,
	issuerKey *ecdsa.PrivateKey,
	modify ...func(*x509.Certificate),
) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "example.org",
		},
		NotBefore:             p.now.Add(-time.Hour),
		NotAfter:              p.now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		CRLDistributionPoints: p.crlURLs,
		OCSPServer:            p.ocspURLs,
		IssuingCertificateURL: p.issuerURLs,
	}
	for _, m := range modify {
		m(template)
	}
	der, err := x509.CreateCertificate(rand.Reader, template, issuer, &key.PublicKey, issuerKey)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
		Leaf:        cert,
	}
}

// thisUpdate is when the PKI's CRLs and OCSP answers were issued.
func (p *testPKI) thisUpdate() time.Time {
	return p.now.Add(-time.Minute)
}

// nextUpdate is when the PKI's CRLs and OCSP answers are due to be replaced.
func (p *testPKI) nextUpdate() time.Time {
	return p.now.Add(time.Hour)
}

// crlTemplate returns a current CRL that lists the PKI's first server certificate when revoked is set.
func (p *testPKI) crlTemplate(revoked bool) *x509.RevocationList {
	template := &x509.RevocationList{
		Number:     big.NewInt(1),
		ThisUpdate: p.thisUpdate(),
		NextUpdate: p.nextUpdate(),
	}
	if revoked {
		template.RevokedCertificateEntries = []x509.RevocationListEntry{{
			SerialNumber:   p.leaf.SerialNumber,
			RevocationTime: p.thisUpdate(),
		}}
	}
	return template
}

// crl returns a DER CRL signed by issuer that lists the PKI's first server certificate when revoked is set.
func (p *testPKI) crl(issuer *x509.Certificate, key *ecdsa.PrivateKey, revoked bool) []byte {
	der, err := x509.CreateRevocationList(rand.Reader, p.crlTemplate(revoked), issuer, key)
	assert.NoError(p.t, err)
	return der
}

// ocspAnswer returns a current answer with status for the PKI's first server certificate.
func (p *testPKI) ocspAnswer(status int) ocsp.Response {
	return ocsp.Response{
		Status:       status,
		SerialNumber: p.leaf.SerialNumber,
		ThisUpdate:   p.thisUpdate(),
		NextUpdate:   p.nextUpdate(),
		RevokedAt:    p.thisUpdate(),
	}
}

// writeOCSP writes answer about a certificate of the CA, signed with key, which signer certifies: the CA itself, a
// responder it delegated to, or a rogue CA.
func (p *testPKI) writeOCSP(
	w http.ResponseWriter,
	signer *x509.Certificate,
	key *ecdsa.PrivateKey,
	answer ocsp.Response,
) {
	p.writeOCSPAbout(w, p.ca, signer, key, answer)
}

// writeOCSPAbout writes answer, signed with key, about a certificate that issuer issued.
func (p *testPKI) writeOCSPAbout(
	w http.ResponseWriter,
	issuer, signer *x509.Certificate,
	key *ecdsa.PrivateKey,
	answer ocsp.Response,
) {
	resp, err := ocsp.CreateResponse(issuer, signer, answer, key)
	if !assert.NoError(p.t, err) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/ocsp-response")
	_, _ = w.Write(resp)
}

// ocspRequestBytes returns the DER request of a POST body, or of a GET, which carries it URL-escaped and base64
// encoded as the last path segment.
func (p *testPKI) ocspRequestBytes(r *http.Request) ([]byte, error) {
	if r.Method == http.MethodPost {
		if ct := r.Header.Get("Content-Type"); ct != "application/ocsp-request" {
			return nil, fmt.Errorf("unexpected content type %q", ct)
		}
		return io.ReadAll(r.Body)
	}
	encoded, err := url.QueryUnescape(path.Base(r.URL.EscapedPath()))
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(encoded)
}

func serveIssuer(p *testPKI, w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write(p.ca.Raw)
}

// serveIssuerWithoutCRLSign serves a certificate for the CA's key that lacks the CRL-signing key usage.
func serveIssuerWithoutCRLSign(p *testPKI, w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write(p.caWithoutCRLSign.Raw)
}

// serveRogueIssuer serves the rogue CA, which has the CA's name but not its key.
func serveRogueIssuer(p *testPKI, w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write(p.rogue.Raw)
}

func serveIssuerPEM(p *testPKI, w http.ResponseWriter, _ *http.Request) {
	_ = pem.Encode(w, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: p.ca.Raw,
	})
}

func serveCRL(revoked bool) testPKIHandler {
	return func(p *testPKI, w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(p.crl(p.ca, p.caKey, revoked))
	}
}

func serveCRLPEM(revoked bool) testPKIHandler {
	return func(p *testPKI, w http.ResponseWriter, _ *http.Request) {
		_ = pem.Encode(w, &pem.Block{
			Type:  "X509 CRL",
			Bytes: p.crl(p.ca, p.caKey, revoked),
		})
	}
}

// serveCRLWith serves a CRL that lists the server certificate, signed by the CA after modify changes its template.
func serveCRLWith(modify func(p *testPKI, rl *x509.RevocationList)) testPKIHandler {
	return func(p *testPKI, w http.ResponseWriter, _ *http.Request) {
		template := p.crlTemplate(true)
		modify(p, template)
		der, err := x509.CreateRevocationList(rand.Reader, template, p.ca, p.caKey)
		if !assert.NoError(p.t, err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(der)
	}
}

// serveRogueCRL serves a CRL that lists the server certificate, signed by the rogue CA.
func serveRogueCRL(p *testPKI, w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write(p.crl(p.rogue, p.rogueKey, true))
}

// serveRenamedCRL serves a CRL that lists the server certificate, signed with the CA's key under another name.
func serveRenamedCRL(p *testPKI, w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write(p.crl(p.caRenamed, p.caKey, true))
}

// serveOCSP answers with status after checking that the request names the server certificate. Like OpenSSL's
// responder, it answers unknown when the request names another issuer.
func serveOCSP(status int) testPKIHandler {
	return func(p *testPKI, w http.ResponseWriter, r *http.Request) {
		der, err := p.ocspRequestBytes(r)
		if !assert.NoError(p.t, err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req, err := ocsp.ParseRequest(der)
		if !assert.NoError(p.t, err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		assert.Equal(p.t, p.leaf.SerialNumber, req.SerialNumber)
		own, err := ocsp.CreateRequest(p.leaf, p.ca, nil)
		if !assert.NoError(p.t, err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ownReq, err := ocsp.ParseRequest(own)
		if !assert.NoError(p.t, err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		answer := p.ocspAnswer(status)
		if !bytes.Equal(req.IssuerNameHash, ownReq.IssuerNameHash) ||
			!bytes.Equal(req.IssuerKeyHash, ownReq.IssuerKeyHash) {
			answer.Status = ocsp.Unknown
		}
		answer.SerialNumber = req.SerialNumber
		p.writeOCSP(w, p.ca, p.caKey, answer)
	}
}

// serveOCSPWith answers good, signed by the CA, after modify changes the answer.
func serveOCSPWith(modify func(p *testPKI, answer *ocsp.Response)) testPKIHandler {
	return func(p *testPKI, w http.ResponseWriter, _ *http.Request) {
		answer := p.ocspAnswer(ocsp.Good)
		modify(p, &answer)
		p.writeOCSP(w, p.ca, p.caKey, answer)
	}
}

// serveOCSPAbout answers good for the server certificate's serial, signed by the CA, but about a certificate that
// issuer issued.
func serveOCSPAbout(issuer func(p *testPKI) *x509.Certificate) testPKIHandler {
	return func(p *testPKI, w http.ResponseWriter, _ *http.Request) {
		p.writeOCSPAbout(w, issuer(p), p.ca, p.caKey, p.ocspAnswer(ocsp.Good))
	}
}

// serveDelegatedOCSP answers good, signed by a responder whose certificate the CA issued for OCSP signing, after
// modify changes the responder certificate's template.
func serveDelegatedOCSP(modify func(p *testPKI, responder *x509.Certificate)) testPKIHandler {
	return serveDelegatedOCSPFrom(
		func(p *testPKI) (*x509.Certificate, *ecdsa.PrivateKey) { return p.ca, p.caKey },
		modify,
	)
}

// serveDelegatedOCSPFrom answers good, signed by a responder whose certificate the CA that certifier returns issued
// for OCSP signing, after modify changes the responder certificate's template.
func serveDelegatedOCSPFrom(
	certifier func(p *testPKI) (*x509.Certificate, *ecdsa.PrivateKey),
	modify func(p *testPKI, responder *x509.Certificate),
) testPKIHandler {
	return func(p *testPKI, w http.ResponseWriter, _ *http.Request) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if !assert.NoError(p.t, err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		template := &x509.Certificate{
			SerialNumber: big.NewInt(100),
			Subject: pkix.Name{
				CommonName: "Test CA OCSP Responder",
			},
			NotBefore:   p.now.Add(-time.Hour),
			NotAfter:    p.now.Add(time.Hour),
			KeyUsage:    x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageOCSPSigning},
		}
		modify(p, template)
		ca, caKey := certifier(p)
		der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
		if !assert.NoError(p.t, err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		responder, err := x509.ParseCertificate(der)
		if !assert.NoError(p.t, err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		answer := p.ocspAnswer(ocsp.Good)
		answer.Certificate = responder
		p.writeOCSP(w, responder, key, answer)
	}
}

func serveRogueOCSP(p *testPKI, w http.ResponseWriter, _ *http.Request) {
	p.writeOCSP(w, p.rogue, p.rogueKey, p.ocspAnswer(ocsp.Good))
}

// serveOCSPForAnotherCertificate answers good, signed by the CA, for a serial other than the requested one.
func serveOCSPForAnotherCertificate(p *testPKI, w http.ResponseWriter, _ *http.Request) {
	answer := p.ocspAnswer(ocsp.Good)
	answer.SerialNumber = new(big.Int).Add(p.leaf.SerialNumber, big.NewInt(1))
	p.writeOCSP(w, p.ca, p.caKey, answer)
}

func notFound(_ *testPKI, w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func unexpected(p *testPKI, w http.ResponseWriter, r *http.Request) {
	p.t.Errorf("unexpected request to %s", r.URL)
	http.NotFound(w, r)
}

// stall holds the request until the client gives up or the test ends.
func stall(p *testPKI, _ http.ResponseWriter, r *http.Request) {
	select {
	case <-r.Context().Done():
	case <-p.t.Context().Done():
	}
}

// cancelAndStall cancels the caller's context once its request arrives, then stalls.
func cancelAndStall(cancel context.CancelFunc) testPKIHandler {
	return func(p *testPKI, w http.ResponseWriter, r *http.Request) {
		cancel()
		stall(p, w, r)
	}
}
