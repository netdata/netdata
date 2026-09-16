// SPDX-License-Identifier: GPL-3.0-or-later

package redfishsd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func decodeDiscoveryConfig(t *testing.T, input string) Config {
	t.Helper()
	var cfg Config
	require.NoError(t, json.Unmarshal([]byte(input), &cfg))
	return cfg
}

func TestConfigDefaultsAndExplicitZero(t *testing.T) {
	base := `{
		"profiles":[{"name":"p","scheme":"http","auth_method":"none"}],
		"networks":[{"subnet":"192.0.2.1","ports":[80],"profile":"p"}]
	}`
	parsed, err := decodeDiscoveryConfig(t, base).validateAndParse()
	require.NoError(t, err)
	require.Equal(t, 30*time.Minute, parsed.rescanInterval)
	require.Equal(t, 12*time.Hour, parsed.deviceCacheTTL)
	require.False(t, parsed.scanOnce)
	require.Equal(t, "system_vnodes", parsed.profiles["p"].JobConfig["node_mode"])

	withZero := `{
		"rescan_interval":"0s",
		"device_cache_ttl":"0s",
		"profiles":[{"name":"p","scheme":"http","auth_method":"none"}],
		"networks":[{"subnet":"192.0.2.1","ports":[80],"profile":"p"}]
	}`
	parsed, err = decodeDiscoveryConfig(t, withZero).validateAndParse()
	require.NoError(t, err)
	require.True(t, parsed.scanOnce)
	require.Zero(t, parsed.rescanInterval)
	require.Zero(t, parsed.deviceCacheTTL)
}

func TestConfigAcceptsOperatorSelectedConcurrencyAboveDefaultSafetyRanges(t *testing.T) {
	cfg := decodeDiscoveryConfig(t, `{
		"max_concurrent_scans":1024,
		"profiles":[{"name":"p","scheme":"http","auth_method":"none"}],
		"networks":[{"subnet":"192.0.2.1","ports":[80],"profile":"p"}]
	}`)
	parsed, err := cfg.validateAndParse()
	require.NoError(t, err)
	require.Equal(t, 1024, parsed.maxConcurrentScans)
}

func TestConfigRejectsNonStringProfileIdentityFields(t *testing.T) {
	for name, input := range map[string]string{
		"name":   `{"profiles":[{"name":1,"auth_method":"none"}]}`,
		"scheme": `{"profiles":[{"name":"p","scheme":443,"auth_method":"none"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			var cfg Config
			require.Error(t, json.Unmarshal([]byte(input), &cfg))
		})
	}
}

func TestConfigPreservesSupportedGeneratedJobOptions(t *testing.T) {
	cfg := decodeDiscoveryConfig(t, `{
		"profiles":[{
			"name":"p",
			"scheme":"https",
			"node_mode":"local",
			"vnode":"site-bmc",
			"auth_method":"none",
			"autodetection_retry":30,
			"charts":{"aggregates":true,"details":false},
			"alarms":{"evaluate_thresholds":false},
			"logs":{"enabled":false}
		}],
		"networks":[{"subnet":"192.0.2.1","ports":[443],"profile":"p"}]
	}`)

	parsed, err := cfg.validateAndParse()
	require.NoError(t, err)
	profile := parsed.profiles["p"].JobConfig
	require.Equal(t, "local", profile["node_mode"])
	require.Equal(t, "site-bmc", profile["vnode"])
	require.Equal(t, json.Number("30"), profile["autodetection_retry"])
	require.Equal(t, map[string]any{
		"aggregates": true,
		"details":    false,
	}, profile["charts"])
	require.Equal(t, map[string]any{"evaluate_thresholds": false}, profile["alarms"])
	require.Equal(t, map[string]any{"enabled": false}, profile["logs"])
}

func TestConfigKeepsEndpointCredentialsOutOfDiscoveryProbe(t *testing.T) {
	cfg := decodeDiscoveryConfig(t, `{
		"profiles":[{
			"name":"p",
			"scheme":"https",
			"auth_method":"basic",
			"username":"monitoring",
			"password":"test-password",
			"tls_cert":"/example/client.pem",
			"tls_key":"/example/client-key.pem",
			"proxy_url":"http://proxy.example.test:8080"
		}],
		"networks":[{"subnet":"192.0.2.1","ports":[443],"profile":"p"}]
	}`)

	parsed, err := cfg.validateAndParse()
	require.NoError(t, err)
	profile := parsed.profiles["p"]
	for _, key := range []string{"username", "password", "tls_cert", "tls_key"} {
		require.Contains(t, profile.JobConfig, key)
		require.NotContains(t, profile.ProbeConfig, key)
	}
	require.Equal(t, "http://proxy.example.test:8080", profile.ProbeConfig["proxy_url"])
}

func TestConfigRedactsProxySecretResolverFailure(t *testing.T) {
	const reference = "PRIVATE_PROXY_REFERENCE_OPERAND"

	cfg := decodeDiscoveryConfig(t, `{
		"profiles":[{
			"name":"p",
			"scheme":"https",
			"auth_method":"none",
			"proxy_url":"${unknown:`+reference+`}"
		}],
		"networks":[{"subnet":"192.0.2.1","ports":[443],"profile":"p"}]
	}`)

	_, err := cfg.validateAndParse()
	require.Error(t, err)
	require.Contains(t, err.Error(), "proxy_url secret reference could not be resolved")
	require.NotContains(t, strings.ToLower(err.Error()), strings.ToLower(reference))
}

func TestConfigRejectsConflictingEqualPrefixAndLargeNetwork(t *testing.T) {
	conflict := `{
		"profiles":[
			{"name":"a","scheme":"http","auth_method":"none"},
			{"name":"b","scheme":"http","auth_method":"none"}
		],
		"networks":[
			{"subnet":"192.0.2.1","ports":[80],"profile":"a"},
			{"subnet":"192.0.2.1/32","ports":[443],"profile":"b"}
		]
	}`
	_, err := decodeDiscoveryConfig(t, conflict).validateAndParse()
	require.ErrorContains(t, err, "assigned to both profile")

	large := `{
		"profiles":[{"name":"p","scheme":"http","auth_method":"none"}],
		"networks":[{"subnet":"198.18.0.0/22","ports":[80],"profile":"p"}]
	}`
	_, err = decodeDiscoveryConfig(t, large).validateAndParse()
	require.ErrorContains(t, err, "512-address maximum")

	largeIPv6 := `{
		"profiles":[{"name":"p","scheme":"http","auth_method":"none"}],
		"networks":[{"subnet":"2001:db8::/64","ports":[80],"profile":"p"}]
	}`
	_, err = decodeDiscoveryConfig(t, largeIPv6).validateAndParse()
	require.ErrorContains(t, err, "512-address maximum")
	require.ErrorContains(t, err, "18446744073709551614 addresses")
}

func TestConfigCanonicalizesIPv4MappedNetworks(t *testing.T) {
	cfg := decodeDiscoveryConfig(t, `{
		"profiles":[{"name":"p","scheme":"https","auth_method":"none"}],
		"networks":[
			{"subnet":"192.0.2.0/31","ports":[443],"profile":"p"},
			{"subnet":"::ffff:192.0.2.0/127","ports":[8443],"profile":"p"},
			{"subnet":"::ffff:192.0.2.2","ports":[9443],"profile":"p"}
		]
	}`)
	parsed, err := cfg.validateAndParse()
	require.NoError(t, err)
	require.Len(t, parsed.networks, 2)
	byPrefix := make(map[string]network, len(parsed.networks))
	for _, value := range parsed.networks {
		byPrefix[value.normalized] = value
	}
	require.Equal(t, []int{443, 8443}, byPrefix["192.0.2.0/31"].ports)
	require.Contains(t, byPrefix, "192.0.2.2/32")

	_, err = normalizeNetwork("::ffff:192.0.2.0/95")
	require.ErrorContains(t, err, "must be /96 or narrower")
}

func TestEffectiveCandidatesUseMostSpecificNetwork(t *testing.T) {
	cfg := decodeDiscoveryConfig(t, `{
		"profiles":[
			{"name":"broad","scheme":"http","auth_method":"none"},
			{"name":"specific","scheme":"https","auth_method":"none"}
		],
		"networks":[
			{"subnet":"192.0.2.0/30","ports":[80],"profile":"broad"},
			{"subnet":"192.0.2.1/32","ports":[8443],"profile":"specific"}
		]
	}`)
	discoverer, err := NewDiscoverer(cfg)
	require.NoError(t, err)
	candidates := discoverer.effectiveCandidates()
	require.Len(t, candidates, 2)
	byAddress := make(map[string]endpointCandidate)
	for _, candidate := range candidates {
		byAddress[candidate.address.String()] = candidate
	}
	require.Equal(t, "specific", byAddress["192.0.2.1"].profile.Name)
	require.Equal(t, 8443, byAddress["192.0.2.1"].port)
	require.Equal(t, "broad", byAddress["192.0.2.2"].profile.Name)
	require.Equal(t, 80, byAddress["192.0.2.2"].port)
}

func TestVisitEffectiveCandidatesStopsAtConsumerBoundary(t *testing.T) {
	cfg := decodeDiscoveryConfig(t, `{
		"profiles":[{"name":"p","scheme":"https","auth_method":"none"}],
		"networks":[{"subnet":"192.0.2.1","ports":[443,8443,9443,10443,11443],"profile":"p"}]
	}`)
	discoverer, err := NewDiscoverer(cfg)
	require.NoError(t, err)

	visited := 0
	completed := discoverer.visitEffectiveCandidates(func(endpointCandidate) bool {
		visited++
		return visited < 3
	})
	require.Equal(t, 3, visited)
	require.False(t, completed)
}

func TestEffectiveCandidatesSupportIPv6LiteralsAndPrefixes(t *testing.T) {
	cfg := decodeDiscoveryConfig(t, `{
		"profiles":[{"name":"p","scheme":"https","auth_method":"none"}],
		"networks":[
			{"subnet":"2001:db8::10","ports":[443],"profile":"p"},
			{"subnet":"2001:db8::20/127","ports":[8443],"profile":"p"}
		]
	}`)
	discoverer, err := NewDiscoverer(cfg)
	require.NoError(t, err)
	candidates := discoverer.effectiveCandidates()
	require.Len(t, candidates, 3)
	require.Equal(t, "https://[2001:db8::10]/redfish/v1/", candidates[0].url)
	require.Equal(t, "https://[2001:db8::20]:8443/redfish/v1/", candidates[1].url)
	require.Equal(t, "https://[2001:db8::21]:8443/redfish/v1/", candidates[2].url)
}

func TestEffectiveCandidatesPreserveZonedIPv6Literals(t *testing.T) {
	cfg := decodeDiscoveryConfig(t, `{
		"profiles":[{"name":"p","scheme":"https","auth_method":"none"}],
		"networks":[
			{"subnet":"fe80::1%Eth0","ports":[443],"profile":"p"},
			{"subnet":"fe80::1%eth0","ports":[8443],"profile":"p"}
		]
	}`)
	discoverer, err := NewDiscoverer(cfg)
	require.NoError(t, err)
	candidates := discoverer.effectiveCandidates()
	require.Len(t, candidates, 2)
	byZone := make(map[string]endpointCandidate, len(candidates))
	for _, candidate := range candidates {
		byZone[candidate.address.Zone()] = candidate
	}
	require.Equal(t, "https://[fe80::1%25Eth0]/redfish/v1/", byZone["Eth0"].url)
	require.Equal(t, "https://[fe80::1%25eth0]:8443/redfish/v1/", byZone["eth0"].url)
	require.Equal(t, "fe80::1%Eth0", newTarget(byZone["Eth0"]).IPAddress)
	require.NotEqual(t, byZone["Eth0"].key, byZone["eth0"].key)
}

func TestConfigRejectsUnzonedLinkLocalIPv6Discovery(t *testing.T) {
	for _, subnet := range []string{"fe80::1", "fe80::/126"} {
		cfg := decodeDiscoveryConfig(t, `{
			"profiles":[{"name":"p","scheme":"https","auth_method":"none"}],
			"networks":[{"subnet":"`+subnet+`","ports":[443],"profile":"p"}]
		}`)
		_, err := cfg.validateAndParse()
		require.ErrorContains(t, err, "link-local IPv6 discovery requires a zoned literal address")
	}
}

func TestConfigRejectsZonedIPv4MappedIPv6Discovery(t *testing.T) {
	cfg := decodeDiscoveryConfig(t, `{
		"profiles":[{"name":"p","scheme":"https","auth_method":"none"}],
		"networks":[{"subnet":"::ffff:192.0.2.1%eth0","ports":[443],"profile":"p"}]
	}`)
	_, err := cfg.validateAndParse()
	require.ErrorContains(t, err, "IPv4 address must not contain an interface zone")
}
