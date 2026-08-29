// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostScopeModesAndDeterministicSystemIdentity(t *testing.T) {
	system := &graphNode{
		Kind: "system",
		Key:  "system-key",
		URI:  "/redfish/v1/Systems/1",
		Doc: genericResource{
			Name: "System One",
		},
		Data: map[string]any{
			"Manufacturer": "Example Vendor",
			"Model":        "Example Model",
			"SerialNumber": "SYNTHETIC-SERIAL",
			"UUID":         "00000000-0000-4000-8000-000000000001",
		},
	}
	system.SystemOwners = map[string]*graphNode{system.Key: system}
	component := &graphNode{
		Kind:         "fan",
		Key:          "fan-key",
		SystemOwners: map[string]*graphNode{system.Key: system},
	}

	local := hostScopeTestClient(t, "https://bmc.example.test", "local", "local-job")
	assert.Empty(t, local.scopeForNode(system))
	assert.Empty(t, local.scopeForNode(component))

	first := hostScopeTestClient(t, "https://bmc.example.test", "system_vnodes", "collector-a")
	second := hostScopeTestClient(t, "https://bmc.example.test", "system_vnodes", "collector-b")
	firstScope := first.scopeForNode(component)
	secondScope := second.scopeForNode(component)
	require.NotEmpty(t, firstScope.GUID)
	assert.Equal(t, firstScope.GUID, firstScope.ScopeKey)
	assert.Equal(t, firstScope.GUID, secondScope.GUID)
	assert.Equal(t, firstScope.GUID, first.scopeForNode(system).GUID)
	assert.Equal(t, "redfish", firstScope.Labels["_vnode_type"])
	assert.Equal(t, "system-key", firstScope.Labels["system_key"])
	assert.Equal(t, "System One", firstScope.Labels["system_name"])
	assert.Equal(t, "Example Vendor", firstScope.Labels["manufacturer"])
	assert.Equal(t, "SYNTHETIC-SERIAL", firstScope.Labels["serial_number"])
	assert.Equal(t, "collector-a", firstScope.Labels["endpoint_job"])
	assert.Equal(t, "collector-b", secondScope.Labels["endpoint_job"])

	otherEndpoint := hostScopeTestClient(t, "https://other-bmc.example.test", "system_vnodes", "collector-a")
	assert.NotEqual(t, firstScope.GUID, otherEndpoint.scopeForNode(component).GUID)
}

func TestHostScopeOverridesAndServiceFallback(t *testing.T) {
	client := hostScopeTestClient(t, "https://bmc.example.test", "system_vnodes", "collector-a")
	client.config.HostScopeOverrides = []HostScopeOverride{
		{
			ResourceURI: "/redfish/v1/Systems/1",
			GUID:        "00000000-0000-4000-8000-000000000010",
			Hostname:    "system-one",
		},
		{
			ResourceURI: "/redfish/v1/",
			GUID:        "00000000-0000-4000-8000-000000000020",
			Hostname:    "service-root",
		},
	}
	system := &graphNode{
		Kind: "system", Key: "system-key", URI: "/redfish/v1/Systems/1",
		Doc: genericResource{Name: "System One"},
	}
	system.SystemOwners = map[string]*graphNode{system.Key: system}
	component := &graphNode{
		Kind:         "fan",
		Key:          "fan-key",
		SystemOwners: map[string]*graphNode{system.Key: system},
	}

	systemScope := client.scopeForNode(component)
	assert.Equal(t, "00000000-0000-4000-8000-000000000010", systemScope.GUID)
	assert.Equal(t, "system-one", systemScope.Hostname)

	orphan := &graphNode{Kind: "manager", Key: "manager-key"}
	serviceScope := client.scopeForNode(orphan)
	assert.Equal(t, "00000000-0000-4000-8000-000000000020", serviceScope.GUID)
	assert.Equal(t, "service-root", serviceScope.Hostname)
	assert.Equal(t, "redfish", serviceScope.Labels["_vnode_type"])
	assert.Equal(t, client.origin, serviceScope.Labels["redfish_origin"])
}

func TestHostScopeIdentityValidationRejectsOverrideCollision(t *testing.T) {
	client := hostScopeTestClient(t, "https://bmc.example.test", "system_vnodes", "collector-a")
	first := &graphNode{
		Kind: "system", Key: "system-key-1", URI: "/redfish/v1/Systems/1",
		Doc: genericResource{Name: "System One"},
	}
	second := &graphNode{
		Kind: "system", Key: "system-key-2", URI: "/redfish/v1/Systems/2",
		Doc: genericResource{Name: "System Two"},
	}
	first.SystemOwners = map[string]*graphNode{first.Key: first}
	second.SystemOwners = map[string]*graphNode{second.Key: second}
	client.config.HostScopeOverrides = []HostScopeOverride{{
		ResourceURI: first.URI,
		GUID:        client.systemHostScope(second).GUID,
	}}

	err := client.validateHostScopeIdentities([]*graphNode{first, second})
	require.ErrorContains(t, err, "Redfish HostScope GUID collision")
	require.ErrorContains(t, err, "system:system-key-1")
	require.ErrorContains(t, err, "system:system-key-2")
}

func hostScopeTestClient(
	t *testing.T,
	rawURL, mode, job string,
) *protocolClient {
	t.Helper()
	root, origin, err := normalizeServiceRoot(rawURL)
	require.NoError(t, err)
	return &protocolClient{
		config:      Config{NodeMode: mode},
		root:        root,
		origin:      origin,
		endpointJob: job,
	}
}
