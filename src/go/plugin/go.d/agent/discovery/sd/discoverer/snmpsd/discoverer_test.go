// SPDX-License-Identifier: GPL-3.0-or-later

package snmpsd

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"
)

func TestNewDiscoverer(t *testing.T) {
	tests := map[string]struct {
		cfg      Config
		wantFail bool
	}{
		"succeeds with valid SNMPv1 config": {
			wantFail: false,
			cfg: Config{
				Credentials: []CredentialConfig{
					{Name: "v1cred", Version: "1", Community: "public"},
				},
				Networks: []NetworkConfig{
					{Subnet: "192.0.2.0/24", Credential: "v1cred"},
				},
			},
		},
		"succeeds with valid SNMPv2c config": {
			wantFail: false,
			cfg: Config{
				Credentials: []CredentialConfig{
					{Name: "v2cred", Version: "2c", Community: "public"},
				},
				Networks: []NetworkConfig{
					{Subnet: "192.0.2.0/24", Credential: "v2cred"},
				},
			},
		},
		"succeeds with valid SNMPv3 config": {
			wantFail: false,
			cfg: Config{
				Credentials: []CredentialConfig{
					{
						Name:              "v3cred",
						Version:           "3",
						UserName:          "user",
						SecurityLevel:     "authPriv",
						AuthProtocol:      "sha",
						AuthPassphrase:    "authpass",
						PrivacyProtocol:   "aes",
						PrivacyPassphrase: "privpass",
					},
				},
				Networks: []NetworkConfig{
					{Subnet: "192.0.2.0/24", Credential: "v3cred"},
				},
			},
		},
		"succeeds with multiple valid credentials and networks": {
			cfg: Config{
				Credentials: []CredentialConfig{
					{Name: "v1cred", Version: "1", Community: "public"},
					{Name: "v2cred", Version: "2c", Community: "private"},
					{
						Name:              "v3cred",
						Version:           "3",
						UserName:          "user",
						SecurityLevel:     "authPriv",
						AuthProtocol:      "sha",
						AuthPassphrase:    "authpass",
						PrivacyProtocol:   "aes",
						PrivacyPassphrase: "privpass",
					},
				},
				Networks: []NetworkConfig{
					{Subnet: "192.0.2.0/24", Credential: "v1cred"},
					{Subnet: "10.0.0.0/24", Credential: "v2cred"},
					{Subnet: "172.16.0.0/24", Credential: "v3cred"},
				},
			},
			wantFail: false,
		},
		"fails on empty config": {
			wantFail: true,
		},
		"fails with credentials but no networks": {
			wantFail: true,
			cfg: Config{
				Credentials: []CredentialConfig{
					{Name: "test", Version: "2c", Community: "public"},
				},
			},
		},
		"fails with networks but no credentials": {
			wantFail: true,
			cfg: Config{
				Networks: []NetworkConfig{
					{Subnet: "192.0.2.0/24", Credential: "test"},
				},
			},
		},
		"fails with credential without name": {
			wantFail: true,
			cfg: Config{
				Credentials: []CredentialConfig{
					{Version: "2c", Community: "public"},
				},
				Networks: []NetworkConfig{
					{Subnet: "192.0.2.0/24", Credential: "test"},
				},
			},
		},
		"fails with duplicate credential names": {
			wantFail: true,
			cfg: Config{
				Credentials: []CredentialConfig{
					{Name: "test", Version: "2c", Community: "public"},
					{Name: "test", Version: "2c", Community: "private"},
				},
				Networks: []NetworkConfig{
					{Subnet: "192.0.2.0/24", Credential: "test"},
				},
			},
		},
		"fails with network without subnet": {
			wantFail: true,
			cfg: Config{
				Credentials: []CredentialConfig{
					{Name: "test", Version: "2c", Community: "public"},
				},
				Networks: []NetworkConfig{
					{Credential: "test"},
				},
			},
		},
		"fails with network without credential": {
			wantFail: true,
			cfg: Config{
				Credentials: []CredentialConfig{
					{Name: "test", Version: "2c", Community: "public"},
				},
				Networks: []NetworkConfig{
					{Subnet: "192.0.2.0/24"},
				},
			},
		},
		"fails with network with nonexistent credential": {
			wantFail: true,
			cfg: Config{
				Credentials: []CredentialConfig{
					{Name: "test", Version: "2c", Community: "public"},
				},
				Networks: []NetworkConfig{
					{Subnet: "192.0.2.0/24", Credential: "nonexistent"},
				},
			},
		},
		"fails with invalid subnet format": {
			wantFail: true,
			cfg: Config{
				Credentials: []CredentialConfig{
					{Name: "test", Version: "2c", Community: "public"},
				},
				Networks: []NetworkConfig{
					{Subnet: "invalid-subnet", Credential: "test"},
				},
			},
		},
		"fails with subnet too large (> 512 IPs)": {
			wantFail: true,
			cfg: Config{
				Credentials: []CredentialConfig{
					{Name: "test", Version: "2c", Community: "public"},
				},
				Networks: []NetworkConfig{
					{Subnet: "192.0.2.0/22", Credential: "test"}, // 1024 IPs
				},
			},
		},
		"fails with duplicate subnet": {
			wantFail: true,
			cfg: Config{
				Credentials: []CredentialConfig{
					{Name: "test", Version: "2c", Community: "public"},
				},
				Networks: []NetworkConfig{
					{Subnet: "192.0.2.0/24", Credential: "test"},
					{Subnet: "192.0.2.0/24", Credential: "test"},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			d, err := NewDiscoverer(test.cfg)

			if test.wantFail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, d)
			}
		})
	}
}

func TestDiscoverer_Run(t *testing.T) {
	tests := map[string]struct {
		prepareSim func(t *testing.T) *discoverySim
	}{
		"simple discovery": {
			prepareSim: func(t *testing.T) *discoverySim {
				cfg := Config{
					Credentials: []CredentialConfig{
						{Name: "public-v2", Version: "2", Community: "public-v2"},
					},
					Networks: []NetworkConfig{
						{Subnet: "192.0.2.0/29", Credential: "public-v2"},
					},
				}

				subnets, err := cfg.validateAndParse()
				sub := subnets[0]
				require.NoError(t, err)

				sim := discoverySim{
					cfg: cfg,
					updateSnmpHandler: func(m *mockSnmpHandler) {
						m.skipOnConnect = func(ip string) bool {
							// Skip if the last octet is odd
							i := strings.LastIndexByte(ip, '.')
							if i == -1 {
								return false
							}
							lastOctet, err := strconv.Atoi(ip[i+1:])
							return err == nil && lastOctet%2 != 0
						}
					},
					wantGroups: []model.TargetGroup{
						prepareNewTargetGroup(sub, "192.0.2.2", "192.0.2.4", "192.0.2.6"),
					},
				}

				return &sim
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.prepareSim(t)
			sim.run(t)
		})
	}
}

func prepareNewTargetGroup(sub subnet, ips ...string) *targetGroup {
	tgg := newTargetGroup(sub)
	for _, ip := range ips {
		tg := prepareNewTarget(sub, ip)
		tgg.addTarget(tg)
	}
	return tgg
}

func prepareNewTarget(sub subnet, ip string) *target {
	return newTarget(ip, sub.credential, SysInfo{
		Descr:        mockSysDescr,
		Contact:      mockSysContact,
		Name:         mockSysName,
		Location:     mockSysLocation,
		Organization: "net-snmp",
	})
}
