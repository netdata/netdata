// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtocolClientURIProfiles(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))

	for name, raw := range map[string]string{
		"fragment":             "/redfish/v1/Systems/1#/Status",
		"encoded dot":          "/redfish/v1/%2e%2e/admin",
		"encoded slash":        "/redfish/v1/Systems%2f1",
		"encoded unreserved":   "/redfish/v1/Systems/%41",
		"backslash":            `/redfish/v1\\admin`,
		"path relative":        "Systems/1",
		"ordinary query":       "/redfish/v1/Systems?$top=1",
		"cross origin":         "https://example.com/redfish/v1/Systems",
		"outside Redfish path": "/admin",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := client.resolveURI(client.root, raw, false)
			require.Error(t, err)
		})
	}
	opaque, err := client.resolveURI(client.root, "/redfish/v1/Systems?$skip=1", true)
	require.NoError(t, err)
	assert.Equal(t, "$skip=1", opaque.RawQuery)

	provenance, err := resolveRedfishURI(
		client.origin,
		client.root,
		"/redfish/v1/Chassis/1/Sensors/1#/Reading",
		uriProvenance,
	)
	require.NoError(t, err)
	assert.Equal(t, "/redfish/v1/Chassis/1/Sensors/1#/Reading", canonicalProvenanceURI(provenance))
	provenance, err = resolveRedfishURI(
		client.origin,
		provenance,
		"#/ReadingCelsius",
		uriProvenance,
	)
	require.NoError(t, err)
	assert.Equal(
		t,
		"/redfish/v1/Chassis/1/Sensors/1#/ReadingCelsius",
		canonicalProvenanceURI(provenance),
	)
	_, err = resolveRedfishURI(client.origin, client.root, "#/Status", uriResource)
	require.Error(t, err)
	_, err = resolveRedfishURI(
		client.origin,
		client.root,
		"/redfish/v1/Chassis/1/Sensors/1#not-a-pointer",
		uriProvenance,
	)
	require.Error(t, err)

	root, origin, err := NormalizeServiceRoot("https://bmc.example.test.")
	require.NoError(t, err)
	target, err := resolveRedfishURI(
		origin,
		root,
		"https://BMC.EXAMPLE.TEST./redfish/v1/Systems/1",
		uriResource,
	)
	require.NoError(t, err)
	assert.Equal(t, "https://bmc.example.test/redfish/v1/Systems/1", target.String())
}

func TestNormalizeServiceRoot(t *testing.T) {
	root, origin, err := NormalizeServiceRoot("https://BMC.Example.TEST.:443/redfish/v1")
	require.NoError(t, err)
	require.Equal(t, "https://bmc.example.test", origin)
	require.Equal(t, "https://bmc.example.test/redfish/v1/", root.String())

	root, origin, err = NormalizeServiceRoot("https://[fe80::1%25eno1]:443/")
	require.NoError(t, err)
	require.Equal(t, "https://[fe80::1%25eno1]", origin)
	require.Equal(t, "https://[fe80::1%25eno1]/redfish/v1/", root.String())

	root, origin, err = NormalizeServiceRoot("http://BMC.Example.TEST:080/redfish/v1/")
	require.NoError(t, err)
	require.Equal(t, "http://bmc.example.test", origin)
	require.Equal(t, "http://bmc.example.test/redfish/v1/", root.String())

	_, _, err = NormalizeServiceRoot("https://./redfish/v1/")
	require.ErrorContains(t, err, "host is required")

	_, _, err = NormalizeServiceRoot("https://bmc%25suffix/redfish/v1/")
	require.ErrorContains(t, err, "DNS host must not contain a percent escape")

	_, _, err = NormalizeServiceRoot("https://[::ffff:192.0.2.1%25eth0]/redfish/v1/")
	require.ErrorContains(t, err, "IPv4 host must not contain an interface zone")
}

func TestNormalizeServiceRootCanonicalizesIPv4MappedIPv6(t *testing.T) {
	plainRoot, plainOrigin, err := NormalizeServiceRoot("https://192.0.2.1/redfish/v1/")
	require.NoError(t, err)
	mappedRoot, mappedOrigin, err := NormalizeServiceRoot("https://[::ffff:192.0.2.1]/redfish/v1/")
	require.NoError(t, err)
	require.Equal(t, plainOrigin, mappedOrigin)
	require.Equal(t, plainRoot.String(), mappedRoot.String())

	plainURL, plainOrigin, err := NormalizeServiceRoot("https://192.0.2.1")
	require.NoError(t, err)
	mappedURL, mappedOrigin, err := NormalizeServiceRoot("https://[::ffff:192.0.2.1]")
	require.NoError(t, err)
	require.Equal(t, plainURL, mappedURL)
	require.Equal(t, plainOrigin, mappedOrigin)
}
