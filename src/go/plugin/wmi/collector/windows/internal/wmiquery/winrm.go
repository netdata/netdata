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

const cimv2Resource = "http://schemas.microsoft.com/wbem/wsman/1/wmi/root/cimv2/*"

type winRMClient struct {
	endpoint  string
	client    *wsman.HTTPTransport
	transport *http.Transport
}

// OpenWinRM creates a poll-scoped client for the remote Windows HTTPS listener.
func OpenWinRM(host, username, password string, tlsConfig *tls.Config) *winRMClient {
	endpoint := url.URL{Scheme: "https", Host: net.JoinHostPort(host, "5986"), Path: "/wsman"}
	return newWinRM(endpoint.String(), username, password, tlsConfig)
}

func newWinRM(endpoint, username, password string, tlsConfig *tls.Config) *winRMClient {
	// A session owns its HTTP/1.1 transport and NTLM identity. The negotiator may
	// retain the first 401 response while starting its handshake on another connection.
	base := &http.Transport{TLSClientConfig: tlsConfig}
	client := wsman.NewHTTPTransport(endpoint, &http.Client{
		Transport: winRMTransport{negotiator: ntlmssp.Negotiator{RoundTripper: base}},
	})
	// Negotiator consumes these credentials locally with Basic authentication disabled.
	client.SetCredentials(username, password)
	return &winRMClient{endpoint: endpoint, client: client, transport: base}
}

type winRMTransport struct {
	negotiator ntlmssp.Negotiator
}

func (t winRMTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.negotiator.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusInternalServerError: // WinRM returns SOAP faults with status 500.
		resp.Body = web.LimitResponseBody(resp.Body, 153600) // MaxEnvelopeSize advertised by go-wsman.
		return resp, nil
	default:
		// In particular, never follow a redirect to another endpoint with these credentials.
		_ = resp.Body.Close()
		return nil, diagnostic(fmt.Sprintf("WinRM returned HTTP %d", resp.StatusCode))
	}
}

func (c *winRMClient) Close() { c.transport.CloseIdleConnections() }

func (c *winRMClient) Query(ctx context.Context, wql string) ([]map[string]any, error) {
	// Windows WinRM rejects numeric XML character references around WQL string
	// literals. Only the fixed query contributes apostrophes to this envelope.
	request, err := wsman.BuildEnumerateRequest(cimv2Resource, c.endpoint, wsman.WithWQL(wql))
	if err != nil {
		return nil, diagnostic("could not encode WinRM enumeration")
	}
	request = bytes.ReplaceAll(request, []byte("&#39;"), []byte("'"))
	data, err := c.client.Send(ctx, request)
	if err != nil {
		return nil, winRMError(ctx, err)
	}
	cursor, err := wsman.ParseEnumerateResponse(data)
	if err != nil {
		return nil, winRMError(ctx, err)
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
		request, err = wsman.BuildPullRequest(cimv2Resource, c.endpoint, escaped.String())
		if err != nil {
			return nil, diagnostic("could not encode WinRM pull")
		}
		data, err = c.client.Send(ctx, request)
		if err != nil {
			return nil, winRMError(ctx, err)
		}
		page, err := wsman.ParsePullResponse(data)
		if err != nil {
			return nil, winRMError(ctx, err)
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

func winRMError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if detail := SafeMessage(err); detail != "" {
		return diagnostic(detail)
	}
	var certErr *tls.CertificateVerificationError
	var fault *wsman.Fault
	switch {
	case errors.As(err, &certErr):
		return diagnostic("WinRM server certificate verification failed")
	case errors.As(err, &fault):
		return diagnostic("WinRM returned a WS-Management fault")
	default:
		return diagnostic("WinRM request failed")
	}
}

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
