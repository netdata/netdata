// SPDX-License-Identifier: GPL-3.0-or-later

package wmiquery

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/Azure/go-ntlmssp"
	"github.com/r4sd/go-wsman/wsman"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const (
	winRMHTTPSPort = "5986"
	cimv2Resource  = "http://schemas.microsoft.com/wbem/wsman/1/wmi/root/cimv2/*"
	// maxEnvelopeBytes matches the MaxEnvelopeSize that go-wsman advertises in every request.
	maxEnvelopeBytes = 153600
)

// WinRMSession is one collection cycle's WMI session over the WinRM HTTPS listener.
type WinRMSession struct {
	endpoint string
	wsman    *wsman.HTTPTransport
	http     *http.Transport
	// failed keeps the first transport failure. A refused connection, an
	// untrusted certificate or a rejected login fails every later request the
	// same way, and repeating a rejected login counts toward account lockout.
	// The session lasts one poll, so caching a rarer failure, such as a dropped
	// keep-alive connection, costs at most the rest of that poll.
	failed error
}

// OpenWinRM creates a session for the WinRM HTTPS listener on host, authenticating
// with NTLMv2. It does not contact the host; the first Query does.
func OpenWinRM(host, username, password string, tlsConfig *tls.Config) *WinRMSession {
	endpoint := url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(host, winRMHTTPSPort),
		Path:   "/wsman",
	}
	return newWinRMSession(endpoint.String(), username, password, tlsConfig)
}

func newWinRMSession(endpoint, username, password string, tlsConfig *tls.Config) *WinRMSession {
	// A session owns its HTTP/1.1 transport and NTLM identity. The negotiator may
	// retain the first 401 response while starting its handshake on another connection.
	base := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	sender := wsman.NewHTTPTransport(endpoint, &http.Client{
		Transport: authenticatedTransport{
			negotiator: ntlmssp.Negotiator{
				RoundTripper: base,
			},
		},
	})
	// The negotiator consumes these credentials locally; Basic authentication is never sent.
	sender.SetCredentials(username, password)
	return &WinRMSession{
		endpoint: endpoint,
		wsman:    sender,
		http:     base,
	}
}

// authenticatedTransport authenticates with NTLM and accepts only the statuses
// that carry a WS-Management envelope.
type authenticatedTransport struct {
	negotiator ntlmssp.Negotiator
}

func (t authenticatedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.negotiator.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusInternalServerError: // WinRM returns SOAP faults with status 500.
		resp.Body = web.LimitResponseBody(resp.Body, maxEnvelopeBytes)
		return resp, nil
	default:
		// In particular, never follow a redirect to another endpoint with these credentials.
		_ = resp.Body.Close()
		return nil, diagnostic(fmt.Sprintf("WinRM returned HTTP %d", resp.StatusCode))
	}
}

func (s *WinRMSession) Close() { s.http.CloseIdleConnections() }

// Query reads the caller's fixed WQL projection from root/cimv2 with WS-Enumeration.
func (s *WinRMSession) Query(ctx context.Context, wql string) ([]map[string]any, error) {
	// Windows WinRM rejects numeric XML character references around WQL string
	// literals. Only the fixed query contributes apostrophes to this envelope.
	request, err := wsman.BuildEnumerateRequest(cimv2Resource, s.endpoint, wsman.WithWQL(wql))
	if err != nil {
		return nil, diagnostic("could not encode WinRM enumeration")
	}
	request = bytes.ReplaceAll(request, []byte("&#39;"), []byte("'"))
	data, err := s.send(ctx, request)
	if err != nil {
		return nil, err
	}
	cursor, err := wsman.ParseEnumerateResponse(data)
	if err != nil {
		return nil, responseError(ctx, err)
	}
	var rows []map[string]any
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if cursor == "" {
			return nil, diagnostic("WinRM returned an empty enumeration context")
		}
		// BuildPullRequest inserts the opaque cursor as XML element text.
		var escaped bytes.Buffer
		if err := xml.EscapeText(&escaped, []byte(cursor)); err != nil {
			return nil, diagnostic("invalid WinRM enumeration context")
		}
		request, err = wsman.BuildPullRequest(cimv2Resource, s.endpoint, escaped.String())
		if err != nil {
			return nil, diagnostic("could not encode WinRM pull")
		}
		data, err = s.send(ctx, request)
		if err != nil {
			return nil, err
		}
		page, err := wsman.ParsePullResponse(data)
		if err != nil {
			return nil, responseError(ctx, err)
		}
		for _, instance := range page.Items {
			rows = append(rows, winRMRow(instance))
		}
		if page.EndOfSequence {
			return rows, nil
		}
		cursor = page.EnumerationContext
	}
}

// send posts one envelope. go-wsman returns WS-Management faults as data, so
// every error here is a transport failure: connection, TLS, HTTP status or
// reading the response.
func (s *WinRMSession) send(ctx context.Context, request []byte) ([]byte, error) {
	if s.failed != nil {
		return nil, s.failed
	}
	data, err := s.wsman.Send(ctx, request)
	if err == nil {
		return data, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	s.failed = sessionError{transportError(err)}
	return nil, s.failed
}

func transportError(err error) error {
	var certErr *tls.CertificateVerificationError
	switch {
	case SafeMessage(err) != "":
		return diagnostic(SafeMessage(err))
	case errors.As(err, &certErr):
		return diagnostic("WinRM server certificate verification failed")
	case networkFailure(err) != "":
		return diagnostic("WinRM request failed: " + networkFailure(err))
	default:
		return diagnostic("WinRM request failed")
	}
}

func responseError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var fault *wsman.Fault
	if errors.As(err, &fault) {
		return diagnostic("WinRM returned a WS-Management fault")
	}
	return diagnostic("WinRM returned an invalid response")
}

// winRMRow converts WS-Management text values to the value shapes DCOM returns
// for the collector's projections. The XML carries no CIM types, so array and
// reference properties are recognized by name.
func winRMRow(instance *wsman.Instance) map[string]any {
	properties := instance.PropertiesList()
	row := make(map[string]any, len(properties))
	for name, values := range properties {
		key := strings.ToLower(name)
		switch {
		case key == "ipaddress":
			// CIM string[] remains an array when the host reports a single address.
			row[key] = values
		case key == "dependent" && len(values) == 1:
			// Win32_LogicalDiskToPartition.Dependent is a Win32_LogicalDisk
			// reference. Its only selector is DeviceID, which go-wsman exposes as text.
			row[key] = fmt.Sprintf(`Win32_LogicalDisk.DeviceID="%s"`, values[0])
		case len(values) == 1:
			row[key] = values[0]
		}
	}
	return row
}
