// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ocsp"
)

func TestRevocationChecker_Status(t *testing.T) {
	type result struct {
		revoked, ok bool
	}
	tests := map[string]struct {
		endpoints testPKIEndpoints
		at        func(*testPKI) time.Time
		want      result
		wantErr   bool
	}{
		"no CRL or OCSP URL": {
			want: result{
				ok: true,
			},
		},
		"CRL lists the certificate": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveCRL(true)},
			},
			want: result{
				revoked: true,
				ok:      true,
			},
		},
		"CRL does not list the certificate": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveCRL(false)},
			},
			want: result{
				ok: true,
			},
		},
		"PEM CRL and issuer": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuerPEM},
				crls:    []testPKIHandler{serveCRLPEM(true)},
			},
			want: result{
				revoked: true,
				ok:      true,
			},
		},
		"CRL without an issuer is used unverified": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{notFound},
				crls:    []testPKIHandler{serveRogueCRL},
			},
			want: result{
				revoked: true,
				ok:      true,
			},
		},
		"CRL signed by another CA": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveRogueCRL},
			},
			wantErr: true,
		},
		"CRL unreachable": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{notFound},
			},
			wantErr: true,
		},
		"CRL is not a CRL": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveIssuer},
			},
			wantErr: true,
		},
		"LDAP CRL is skipped": {
			endpoints: testPKIEndpoints{
				ldapCRL: true,
			},
			want: result{
				ok: true,
			},
		},
		"later CRL lists the certificate": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveCRL(false), serveCRL(true)},
				ldapCRL: true,
			},
			want: result{
				revoked: true,
				ok:      true,
			},
		},
		"issuer without the CRL-signing key usage": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuerWithoutCRLSign},
				crls:    []testPKIHandler{serveCRL(true)},
			},
			want: result{
				revoked: true,
				ok:      true,
			},
		},
		"revoked by CRL without asking OCSP": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveCRL(true)},
				ocsps:   []testPKIHandler{unexpected},
			},
			want: result{
				revoked: true,
				ok:      true,
			},
		},
		"OCSP good": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveOCSP(ocsp.Good)},
			},
			want: result{
				ok: true,
			},
		},
		"OCSP revoked": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveOCSP(ocsp.Revoked)},
			},
			want: result{
				revoked: true,
				ok:      true,
			},
		},
		"OCSP unknown is reported as revoked": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveOCSP(ocsp.Unknown)},
			},
			want: result{
				revoked: true,
				ok:      true,
			},
		},
		"OCSP after a CRL that does not list the certificate": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				crls:    []testPKIHandler{serveCRL(false)},
				ocsps:   []testPKIHandler{serveOCSP(ocsp.Revoked)},
			},
			want: result{
				revoked: true,
				ok:      true,
			},
		},
		"failing OCSP responder is skipped": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{notFound, serveIssuer},
				ocsps:   []testPKIHandler{notFound, serveOCSP(ocsp.Good)},
			},
			want: result{
				ok: true,
			},
		},
		"OCSP responders unreachable": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{notFound, notFound},
			},
			want: result{},
		},
		"OCSP answer signed by another CA": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveRogueOCSP},
			},
			want: result{},
		},
		"OCSP answer for another certificate": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{serveIssuer},
				ocsps:   []testPKIHandler{serveOCSPForAnotherCertificate},
			},
			want: result{},
		},
		"OCSP without an issuer URL": {
			endpoints: testPKIEndpoints{
				ocsps: []testPKIHandler{unexpected},
			},
			want: result{},
		},
		"OCSP with an unreachable issuer": {
			endpoints: testPKIEndpoints{
				issuers: []testPKIHandler{notFound},
				ocsps:   []testPKIHandler{unexpected},
			},
			want: result{},
		},
		"expired certificate": {
			endpoints: testPKIEndpoints{
				crls: []testPKIHandler{unexpected},
			},
			at:      func(p *testPKI) time.Time { return p.leaf.NotAfter.Add(time.Second) },
			wantErr: true,
		},
		"certificate not yet valid": {
			endpoints: testPKIEndpoints{
				crls: []testPKIHandler{unexpected},
			},
			at:      func(p *testPKI) time.Time { return p.leaf.NotBefore.Add(-time.Second) },
			wantErr: true,
		},
		"certificate at its last valid instant": {
			at: func(p *testPKI) time.Time { return p.leaf.NotAfter },
			want: result{
				ok: true,
			},
		},
		"certificate at its first valid instant": {
			at: func(p *testPKI) time.Time { return p.leaf.NotBefore },
			want: result{
				ok: true,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pki := newTestPKI(t, test.endpoints)
			at := pki.now
			if test.at != nil {
				at = test.at(pki)
			}

			revoked, ok, err := newRevocationChecker().status(t.Context(), pki.leaf, at)

			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.want, result{
					revoked: revoked,
					ok:      ok,
				})
			}
			assert.LessOrEqual(t, pki.hits["issuer"].Load(), int64(len(test.endpoints.issuers)),
				"a check requests each issuer URL at most once")
		})
	}
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
		cert          *x509.Certificate
		at            time.Time
		wantRevoked   bool
		wantDownloads downloads
	}{
		{
			name:        "first check downloads",
			cert:        pki.leaf,
			at:          pki.now,
			wantRevoked: true,
			wantDownloads: downloads{
				crl:    1,
				issuer: 1,
			},
		},
		{
			name:        "cached verdict before the next update",
			cert:        pki.leaf,
			at:          pki.crlNextUpdate().Add(-time.Second),
			wantRevoked: true,
			wantDownloads: downloads{
				crl:    1,
				issuer: 1,
			},
		},
		{
			name:        "another serial downloads",
			cert:        other,
			at:          pki.now,
			wantRevoked: false,
			wantDownloads: downloads{
				crl:    2,
				issuer: 2,
			},
		},
		{
			name:        "first serial is still cached",
			cert:        pki.leaf,
			at:          pki.now,
			wantRevoked: true,
			wantDownloads: downloads{
				crl:    2,
				issuer: 2,
			},
		},
		{
			name:        "another serial is cached too",
			cert:        other,
			at:          pki.now,
			wantRevoked: false,
			wantDownloads: downloads{
				crl:    2,
				issuer: 2,
			},
		},
		{
			name:        "the first serial from another issuer downloads",
			cert:        otherIssuer,
			at:          pki.now,
			wantRevoked: true,
			wantDownloads: downloads{
				crl:    3,
				issuer: 3,
			},
		},
		{
			name:        "next update downloads again",
			cert:        pki.leaf,
			at:          pki.crlNextUpdate(),
			wantRevoked: true,
			wantDownloads: downloads{
				crl:    4,
				issuer: 4,
			},
		},
	} {
		revoked, ok, err := rc.status(t.Context(), step.cert, step.at)

		require.NoError(t, err, step.name)
		assert.True(t, ok, step.name)
		assert.Equal(t, step.wantRevoked, revoked, step.name)
		assert.Equal(t, step.wantDownloads, downloads{
			crl:    pki.hits["crl"].Load(),
			issuer: pki.hits["issuer"].Load(),
		}, step.name)
	}
}

func TestRevocationChecker_CancelledIssuerDownloadIsNotAMissingIssuer(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pki := newTestPKI(t, testPKIEndpoints{
		issuers: []testPKIHandler{cancelAndStall(cancel)},
		crls:    []testPKIHandler{serveRogueCRL},
	})
	rc := newRevocationChecker()

	_, _, err := rc.status(ctx, pki.leaf, pki.now)

	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, rc.crls, "an unverified CRL must not be cached")
}

func TestRevocationChecker_RequestsAreBounded(t *testing.T) {
	pki := newTestPKI(t, testPKIEndpoints{
		crls: []testPKIHandler{stall},
	})
	rc := newRevocationChecker()
	require.Positive(t, rc.client.Timeout)
	rc.client.Timeout = 50 * time.Millisecond

	_, _, err := rc.status(t.Context(), pki.leaf, pki.now)

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
					p.writeOCSP(w, p.ca, p.caKey, p.leaf.SerialNumber, ocsp.Good)
				}},
			})

			resp, err := newRevocationChecker().queryOCSP(t.Context(), pki.leaf.OCSPServer[0], ocspReq, pki.leaf, pki.ca)

			require.NoError(t, err)
			assert.Equal(t, ocsp.Good, resp.Status)
		})
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
	rogue            *x509.Certificate
	rogueKey         *ecdsa.PrivateKey
	leaf             *x509.Certificate

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
		rogue:            rogue,
		rogueKey:         rogueKey,
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
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert
}

// issueLeaf issues a server certificate with serial, signed by issuer, that lists the PKI's URLs.
func (p *testPKI) issueLeaf(
	t *testing.T,
	serial *big.Int,
	issuer *x509.Certificate,
	issuerKey *ecdsa.PrivateKey,
) *x509.Certificate {
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
	der, err := x509.CreateCertificate(rand.Reader, template, issuer, &key.PublicKey, issuerKey)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert
}

func (p *testPKI) crlNextUpdate() time.Time {
	return p.now.Add(time.Hour)
}

// crl returns a DER CRL signed by issuer that lists the PKI's first server certificate when revoked is set.
func (p *testPKI) crl(issuer *x509.Certificate, key *ecdsa.PrivateKey, revoked bool) []byte {
	template := &x509.RevocationList{
		Number:     big.NewInt(1),
		ThisUpdate: p.now.Add(-time.Minute),
		NextUpdate: p.crlNextUpdate(),
	}
	if revoked {
		template.RevokedCertificateEntries = []x509.RevocationListEntry{{
			SerialNumber:   p.leaf.SerialNumber,
			RevocationTime: p.now.Add(-time.Minute),
		}}
	}
	der, err := x509.CreateRevocationList(rand.Reader, template, issuer, key)
	assert.NoError(p.t, err)
	return der
}

func (p *testPKI) writeOCSP(
	w http.ResponseWriter,
	issuer *x509.Certificate,
	key *ecdsa.PrivateKey,
	serial *big.Int,
	status int,
) {
	resp, err := ocsp.CreateResponse(issuer, issuer, ocsp.Response{
		Status:       status,
		SerialNumber: serial,
		ThisUpdate:   p.now.Add(-time.Minute),
		NextUpdate:   p.now.Add(time.Hour),
		RevokedAt:    p.now.Add(-time.Minute),
	}, key)
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

// serveRogueCRL serves a CRL that lists the server certificate, signed by the rogue CA.
func serveRogueCRL(p *testPKI, w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write(p.crl(p.rogue, p.rogueKey, true))
}

// serveOCSP answers with status after checking that the request names the server certificate.
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
		p.writeOCSP(w, p.ca, p.caKey, req.SerialNumber, status)
	}
}

func serveRogueOCSP(p *testPKI, w http.ResponseWriter, _ *http.Request) {
	p.writeOCSP(w, p.rogue, p.rogueKey, p.leaf.SerialNumber, ocsp.Good)
}

// serveOCSPForAnotherCertificate answers good, signed by the CA, for a serial other than the requested one.
func serveOCSPForAnotherCertificate(p *testPKI, w http.ResponseWriter, _ *http.Request) {
	p.writeOCSP(w, p.ca, p.caKey, new(big.Int).Add(p.leaf.SerialNumber, big.NewInt(1)), ocsp.Good)
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
