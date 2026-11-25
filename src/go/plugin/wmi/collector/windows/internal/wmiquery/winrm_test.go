// SPDX-License-Identifier: GPL-3.0-or-later

package wmiquery

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/r4sd/go-wsman/wsman"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWinRMQuery(t *testing.T) {
	const (
		query = "SELECT InterfaceIndex, IPAddress FROM Win32_NetworkAdapterConfiguration WHERE Caption = 'Ethernet'"
		env   = `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:n="http://schemas.xmlsoap.org/ws/2004/09/enumeration"><s:Body>`
		end   = "</s:Body></s:Envelope>"
	)
	responses := []string{
		"<n:EnumerateResponse><n:EnumerationContext>one</n:EnumerationContext></n:EnumerateResponse>",
		"<n:PullResponse><n:EnumerationContext>two&amp;next</n:EnumerationContext><n:Items><Nic><InterfaceIndex>5</InterfaceIndex><IPAddress>192.0.2.1</IPAddress></Nic></n:Items></n:PullResponse>",
		"<n:PullResponse><n:EndOfSequence/><n:Items><Nic><InterfaceIndex>9</InterfaceIndex><IPAddress>192.0.2.2</IPAddress><IPAddress>2001:db8::2</IPAddress></Nic></n:Items></n:PullResponse>",
	}
	var calls int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Equal(t, "/wsman", r.URL.Path)
		data, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Contains(t, string(data), cimv2Resource)
		switch calls {
		case 0:
			assert.Contains(t, string(data), query)
			assert.Contains(t, string(data), "/WQL")
		case 1:
			assert.Contains(t, string(data), ">one</n:EnumerationContext>")
		case 2:
			assert.Contains(t, string(data), ">two&amp;next</n:EnumerationContext>")
		default:
			t.Error("unexpected additional request")
			http.Error(w, "", 400)
			return
		}
		payload := responses[calls]
		calls++
		fmt.Fprint(w, env+payload+end)
	}))
	defer server.Close()
	ca := x509.NewCertPool()
	ca.AddCert(server.Certificate())
	client := newWinRM(server.URL+"/wsman", "reader", "synthetic", &tls.Config{RootCAs: ca})
	defer client.Close()
	rows, err := client.Query(t.Context(), query)
	require.NoError(t, err)
	assert.Equal(t, []map[string]any{
		{"interfaceindex": "5", "ipaddress": []string{"192.0.2.1"}},
		{"interfaceindex": "9", "ipaddress": []string{"192.0.2.2", "2001:db8::2"}},
	}, rows)
	assert.Equal(t, 3, calls)
}

func TestWinRMTransportFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		trust   bool
		message string
	}{
		{"refuse Basic authentication", 401, true, "HTTP 401"},
		{"refuse redirected credentials", 307, true, "HTTP 307"},
		{"reject untrusted listener", 200, false, "certificate verification failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Empty(t, r.Header.Get("Authorization"))
				assert.Equal(t, "/wsman", r.URL.Path)
				w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
				w.Header().Set("Location", "/other")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, "sensitive-test-response")
			}))
			defer server.Close()
			ca := x509.NewCertPool()
			if tc.trust {
				ca.AddCert(server.Certificate())
			}
			client := newWinRM(server.URL+"/wsman", "reader", "fixture-secret", &tls.Config{RootCAs: ca})
			defer client.Close()
			_, err := client.Query(t.Context(), "SELECT Name FROM Win32_ComputerSystem")
			require.Error(t, err)
			assert.Contains(t, SafeMessage(err), tc.message)
			assert.NotContains(t, SafeMessage(err), "sensitive-test-response")
		})
	}
}

func TestWinRMDiskAssociation(t *testing.T) {
	// A projected reference is returned as an endpoint reference by Windows.
	const response = `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:n="http://schemas.xmlsoap.org/ws/2004/09/enumeration" xmlns:w="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"><s:Body><n:PullResponse><n:Items><w:XmlFragment>
	<Dependent xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"><a:Address>http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address><a:ReferenceParameters><w:ResourceURI>http://schemas.microsoft.com/wbem/wsman/1/wmi/root/cimv2/Win32_LogicalDisk</w:ResourceURI><w:SelectorSet><w:Selector Name="DeviceID">D:</w:Selector></w:SelectorSet></a:ReferenceParameters></Dependent>
	<EndingAddress>65535</EndingAddress><StartingAddress>32768</StartingAddress>
	</w:XmlFragment></n:Items><n:EndOfSequence/></n:PullResponse></s:Body></s:Envelope>`
	page, err := wsman.ParsePullResponse([]byte(response))
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, map[string]any{"dependent": `Win32_LogicalDisk.DeviceID="D:"`, "startingaddress": "32768", "endingaddress": "65535"}, winRMRow(page.Items[0]))
}
