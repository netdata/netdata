// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/wmi/collector/windows/internal/wmiquery"
)

func TestCollector_GroupConfig(t *testing.T) {
	creator, ok := collectorapi.DefaultRegistry.Lookup("windows")
	require.True(t, ok)
	for name, tc := range map[string]struct {
		yamlGroups  string
		jsonGroups  string
		wantQueries []string
	}{
		"omitted": {
			wantQueries: []string{queryCPU, queryMemory, queryMemoryCounters, queryPagingFiles, queryNUMANodes, querySystem, queryTerminalServices, queryTimeService, queryNetworkInterfaces, queryNetworkAdapters, queryTCPv4, queryTCPv6, queryUDPv4, queryUDPv6, queryDisk, queryPhysicalDisks, querySMBServer, querySMBShares, queryDiskDrives},
		},
		"empty object": {
			yamlGroups:  "collectors: {}\n",
			jsonGroups:  `,"collectors":{}`,
			wantQueries: []string{queryCPU, queryMemory, queryMemoryCounters, queryPagingFiles, queryNUMANodes, querySystem, queryTerminalServices, queryTimeService, queryNetworkInterfaces, queryNetworkAdapters, queryTCPv4, queryTCPv6, queryUDPv4, queryUDPv6, queryDisk, queryPhysicalDisks, querySMBServer, querySMBShares, queryDiskDrives},
		},
		"disable one group": {
			yamlGroups:  "collectors: {cpu: false}\n",
			jsonGroups:  `,"collectors":{"cpu":false}`,
			wantQueries: []string{queryMemory, queryMemoryCounters, queryPagingFiles, queryNUMANodes, querySystem, queryTerminalServices, queryTimeService, queryNetworkInterfaces, queryNetworkAdapters, queryTCPv4, queryTCPv6, queryUDPv4, queryUDPv6, queryDisk, queryPhysicalDisks, querySMBServer, querySMBShares, queryDiskDrives},
		},
		"one group only": {
			yamlGroups:  "collectors: {cpu: false, memory: false, system: true, network: false, disk: false, smb: false, time: false}\n",
			jsonGroups:  `,"collectors":{"cpu":false,"memory":false,"system":true,"network":false,"disk":false,"smb":false,"time":false}`,
			wantQueries: []string{querySystem, queryTerminalServices},
		},
		"SMB only": {
			yamlGroups:  "collectors: {cpu: false, memory: false, system: false, network: false, disk: false, smb: true, time: false}\n",
			jsonGroups:  `,"collectors":{"cpu":false,"memory":false,"system":false,"network":false,"disk":false,"smb":true,"time":false}`,
			wantQueries: []string{querySMBServer, querySMBShares},
		},
		"time synchronization only": {
			yamlGroups:  "collectors: {cpu: false, memory: false, system: false, network: false, disk: false, smb: false, time: true}\n",
			jsonGroups:  `,"collectors":{"cpu":false,"memory":false,"system":false,"network":false,"disk":false,"smb":false,"time":true}`,
			wantQueries: []string{queryTimeService},
		},
		"all disabled": {
			yamlGroups: "collectors: {cpu: false, memory: false, system: false, network: false, disk: false, smb: false, time: false}\n",
			jsonGroups: `,"collectors":{"cpu":false,"memory":false,"system":false,"network":false,"disk":false,"smb":false,"time":false}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			const yamlBase = "host: windows.example.com\nauth: ntlm\nusername: fixture-account\npassword: fixture-secret\n"
			const jsonBase = `{"host":"windows.example.com","auth":"ntlm","username":"fixture-account","password":"fixture-secret"`
			for format, data := range map[string][]byte{
				"yaml": []byte(yamlBase + tc.yamlGroups),
				"json": []byte(jsonBase + tc.jsonGroups + "}"),
			} {
				t.Run(format, func(t *testing.T) {
					// DynCfg parses into the registry's config type and returns YAML.
					cfg := creator.Config()
					if format == "json" {
						require.NoError(t, json.Unmarshal(data, cfg))
					} else {
						require.NoError(t, yaml.Unmarshal(data, cfg))
					}
					rendered, err := yaml.Marshal(cfg)
					require.NoError(t, err)

					// File jobs and rendered DynCfg jobs both decode YAML into
					// an already constructed collector before its lifecycle begins.
					for _, payload := range [][]byte{[]byte(yamlBase + tc.yamlGroups), rendered} {
						c := creator.CreateV2().(*Collector)
						require.NoError(t, yaml.Unmarshal(payload, c))
						session := &fakeSession{
							results: validQueryResults(),
						}
						c.newSession = func(context.Context) (querySession, error) {
							return session, nil
						}
						if len(tc.wantQueries) == 0 {
							assert.ErrorContains(t, c.Init(t.Context()), "at least one collector group")
							assert.Empty(t, session.queries)
							continue
						}

						require.NoError(t, c.Init(t.Context()))
						_, err = collecttest.CollectScalarSeries(c)
						require.NoError(t, err)
						assert.Equal(t, tc.wantQueries, metricQueries(session.queries))
						assert.Equal(t, 1, session.closed)
					}
				})
			}
		})
	}
}

func TestCollector_ValidateConfig(t *testing.T) {
	tests := map[string]struct {
		change    func(*Collector)
		wantError string
	}{
		"winrm":             {change: func(c *Collector) { c.Transport = "winrm" }},
		"empty transport":   {change: func(c *Collector) { c.Transport = "" }},
		"unknown transport": {change: func(c *Collector) { c.Transport = "https" }, wantError: "transport must"},
		"CA with DCOM":      {change: func(c *Collector) { c.TLSCA = "lab-ca.pem" }, wantError: "tls_ca requires"},
		"host with port": {
			change:    func(c *Collector) { c.Host = "windows.example.com:135" },
			wantError: "host must",
		},
		"host URL": {
			change:    func(c *Collector) { c.Host = "https://windows.example.com/" },
			wantError: "host must",
		},
		"IPv6":             {change: func(c *Collector) { c.Host = "2001:db8::1" }},
		"scoped IPv6":      {change: func(c *Collector) { c.Host = "fe80::1%test0" }},
		"missing auth":     {change: func(c *Collector) { c.Auth = "" }, wantError: "auth must"},
		"unknown auth":     {change: func(c *Collector) { c.Auth = "negotiate" }, wantError: "auth must"},
		"missing username": {change: func(c *Collector) { c.Username = "" }, wantError: "username is required"},
		"domain only": {
			change:    func(c *Collector) { c.Username = `EXAMPLE\` },
			wantError: "username is required",
		},
		"UPN without user": {
			change:    func(c *Collector) { c.Username = "@example.com" },
			wantError: "username is required",
		},
		"domain account":   {change: func(c *Collector) { c.Username = `EXAMPLE\monitor` }},
		"UPN account":      {change: func(c *Collector) { c.Username = "monitor@example.com" }},
		"missing password": {change: func(c *Collector) { c.Password = "" }, wantError: "password is required"},
		"zero timeout":     {change: func(c *Collector) { c.Timeout = 0 }, wantError: "timeout must be positive"},
		"unreadable CA": {
			change: func(c *Collector) {
				c.Transport = "winrm"
				c.TLSCA = "/nonexistent/lab-ca.pem"
			},
			wantError: `could not load tls_ca: could not read certificate "/nonexistent/lab-ca.pem"`,
		},
		"KDC with NTLM": {
			change:    func(c *Collector) { c.KDC = []string{"dc1.example.com"} },
			wantError: "kdc requires",
		},
		"Kerberos": {change: kerberos("monitor@example.com")},
		"Kerberos with KDCs": {
			change: func(c *Collector) {
				kerberos("monitor@example.com")(c)
				c.KDC = []string{
					"dc1.example.com",
					"dc2.example.com:8888",
					"192.0.2.10",
					"2001:db8::10",
					"[2001:db8::11]:88",
				}
			},
		},
		"Kerberos over WinRM": {
			change: func(c *Collector) {
				kerberos("monitor@example.com")(c)
				c.Transport = "winrm"
			},
			wantError: "auth kerberos requires the dcom transport",
		},
		"Kerberos to an IPv4 address": {
			change: func(c *Collector) {
				kerberos("monitor@example.com")(c)
				c.Host = "192.0.2.1"
			},
			wantError: "not an IP address",
		},
		"Kerberos to an IPv6 address": {
			change: func(c *Collector) {
				kerberos("monitor@example.com")(c)
				c.Host = "2001:db8::1"
			},
			wantError: "not an IP address",
		},
		"Kerberos down-level name": {change: kerberos(`EXAMPLE\monitor`), wantError: "user@domain"},
		"Kerberos local account":   {change: kerberos("monitor"), wantError: "user@domain"},
		"Kerberos empty label":     {change: kerberos("monitor@example..com"), wantError: "user@domain"},
		"Kerberos space in domain": {change: kerberos("monitor@exa mple.com"), wantError: "user@domain"},
		"Kerberos second at sign":  {change: kerberos("monitor@ops@example.com"), wantError: "user@domain"},
	}
	for _, kdc := range []string{
		"https://dc1.example.com", "dc1.example.com:0", "dc1.example.com:65536", "dc1.example.com:",
		"dc1.example.com:kerberos", "",
	} {
		tests["Kerberos KDC "+strconv.Quote(kdc)] = struct {
			change    func(*Collector)
			wantError string
		}{
			change: func(c *Collector) {
				kerberos("monitor@example.com")(c)
				c.KDC = []string{"dc1.example.com", kdc}
			},
			wantError: "kdc entries must",
		}
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := configuredCollector()
			tc.change(c)
			if tc.wantError == "" {
				require.NoError(t, c.Init(t.Context()))
			} else {
				require.ErrorContains(t, c.Init(t.Context()), tc.wantError)
			}
		})
	}
}

// kerberos switches a job to Kerberos as username.
func kerberos(username string) func(*Collector) {
	return func(c *Collector) {
		c.Auth = "kerberos"
		c.Username = username
	}
}

func TestKerberosOptions(t *testing.T) {
	for name, tc := range map[string]struct {
		username string
		kdc      []string
		wantAuth [2]string
		wantKDC  []string
	}{
		"DNS domain": {
			username: " monitor@ad.example.com ",
			wantAuth: [2]string{"monitor", "AD.EXAMPLE.COM"},
			wantKDC:  []string{},
		},
		"trailing dot and KDCs": {
			username: "monitor@ad.example.com.",
			kdc:      []string{" dc1.ad.example.com ", "dc2.ad.example.com:8888", "192.0.2.10", "2001:db8::10", "[2001:db8::11]:88"},
			wantAuth: [2]string{"monitor", "AD.EXAMPLE.COM"},
			wantKDC: []string{
				"dc1.ad.example.com:88", "dc2.ad.example.com:8888", "192.0.2.10:88", "[2001:db8::10]:88", "[2001:db8::11]:88",
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			account, realm, ok := kerberosPrincipal(tc.username)
			require.True(t, ok)
			assert.Equal(t, tc.wantAuth, [2]string{account, realm})
			kdcs, ok := kdcAddresses(tc.kdc)
			require.True(t, ok)
			assert.Equal(t, tc.wantKDC, kdcs)
		})
	}
}

func TestCollector_DCOMAuthenticator(t *testing.T) {
	ntlm := configuredCollector()
	require.NoError(t, ntlm.Init(t.Context()))
	assert.Equal(t, wmiquery.NewNTLMAuth("fixture-account", "fixture-secret"), ntlm.dcomAuth)

	krb := configuredCollector()
	kerberos("monitor@example.com")(krb)
	require.NoError(t, krb.Init(t.Context()))
	assert.IsType(t, &wmiquery.KerberosAuth{}, krb.dcomAuth)
	krb.Cleanup(t.Context())
}

func TestCollector_GeneratedVnodeIdentity(t *testing.T) {
	var expectedGUID string
	for i, host := range []string{"windows.example.com", " Windows.Example.com. "} {
		c := configuredCollector()
		c.Host = host
		if i == 1 {
			c.Transport = "winrm"
		}
		require.NoError(t, c.Init(t.Context()))
		vnode := c.VirtualNode()
		require.NotNil(t, vnode)
		if expectedGUID == "" {
			expectedGUID = vnode.GUID
			require.NotEmpty(t, expectedGUID)
		}
		assert.Equal(t, expectedGUID, vnode.GUID)
		assert.Equal(t, "windows.example.com", vnode.Hostname)
		assert.Equal(t, map[string]string{"_vnode_type": "wmi", "_os": "windows", "address": c.Host}, vnode.Labels)
	}
}

func TestConfigSchemaAndMetadata(t *testing.T) {
	collecttest.AssertConfigSchemaMatchesMetadataWith(t, "config_schema.json", "metadata.yaml",
		collecttest.ConfigSchemaCheck{
			Defaults: true,
		})
}
