// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils/snmpmock"
)

type topologyClientSettings struct {
	Target         string
	Port           uint16
	Retries        int
	Timeout        time.Duration
	MaxOids        int
	Version        gosnmp.SnmpVersion
	Community      string
	ContextName    string
	SecurityModel  gosnmp.SnmpV3SecurityModel
	MsgFlags       gosnmp.SnmpV3MsgFlags
	SecurityParams *gosnmp.UsmSecurityParameters
}

func topologyClientSettingsOf(h gosnmp.Handler) topologyClientSettings {
	s := topologyClientSettings{
		Target:      h.Target(),
		Port:        h.Port(),
		Retries:     h.Retries(),
		Timeout:     h.Timeout(),
		MaxOids:     h.MaxOids(),
		Version:     h.Version(),
		Community:   h.Community(),
		ContextName: h.ContextName(),
	}
	if s.Version == gosnmp.Version3 {
		s.SecurityModel, s.MsgFlags = h.SecurityModel(), h.MsgFlags()
		s.SecurityParams, _ = h.SecurityParameters().(*gosnmp.UsmSecurityParameters)
	}
	return s
}

func TestNewSNMPHandlerFromDeviceInfo(t *testing.T) {
	base := ddsnmp.DeviceConnectionInfo{
		Hostname:       "192.0.2.10",
		Port:           1161,
		Community:      "fixture-community",
		Retries:        2,
		Timeout:        7,
		MaxOIDs:        30,
		MaxRepetitions: 15,
	}
	v3 := base
	v3.SNMPVersion = "3"
	v3.V3User, v3.V3SecurityLevel, v3.V3ContextName = "fixture-user", "authPriv", "fixture-context"
	v3.V3AuthProto, v3.V3AuthKey, v3.V3PrivProto, v3.V3PrivKey = "sha", "fixture-auth", "aes", "fixture-priv"
	common := topologyClientSettings{Target: "192.0.2.10", Port: 1161, Retries: 2, Timeout: 7 * time.Second, MaxOids: 30}

	tests := map[string]struct {
		dev     ddsnmp.DeviceConnectionInfo
		want    topologyClientSettings
		wantErr bool
	}{
		"SNMPv1": {
			dev: func() ddsnmp.DeviceConnectionInfo { d := base; d.SNMPVersion = "1"; return d }(),
			want: func() topologyClientSettings {
				s := common
				s.Version, s.Community = gosnmp.Version1, "fixture-community"
				return s
			}(),
		},
		"SNMPv2c": {
			dev: func() ddsnmp.DeviceConnectionInfo { d := base; d.SNMPVersion = "2c"; return d }(),
			want: func() topologyClientSettings {
				s := common
				s.Version, s.Community = gosnmp.Version2c, "fixture-community"
				return s
			}(),
		},
		"SNMPv3": {
			dev: v3,
			want: func() topologyClientSettings {
				s := common
				s.Version, s.Community, s.ContextName = gosnmp.Version3, "public", "fixture-context"
				s.SecurityModel, s.MsgFlags = gosnmp.UserSecurityModel, gosnmp.AuthPriv
				s.SecurityParams = &gosnmp.UsmSecurityParameters{
					UserName:                 "fixture-user",
					AuthenticationProtocol:   gosnmp.SHA,
					AuthenticationPassphrase: "fixture-auth",
					PrivacyProtocol:          gosnmp.AES,
					PrivacyPassphrase:        "fixture-priv",
				}
				return s
			}(),
		},
		"SNMPv3 without a user": {
			dev:     func() ddsnmp.DeviceConnectionInfo { d := v3; d.V3User = ""; return d }(),
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			h, err := newSNMPHandlerFromDeviceInfo(tc.dev)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, topologyClientSettingsOf(h))
		})
	}
}

func TestRefreshDeviceTopologyConfiguresClientAndProfileCollector(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:       "192.0.2.10",
		Port:           161,
		Community:      "fixture-community",
		SNMPVersion:    gosnmp.Version2c.String(),
		MaxOIDs:        30,
		MaxRepetitions: 15,
	}
	session := snmpmock.NewMockSession(ctrl)
	expectTopologyRefreshSNMPClient(session, dev)

	coll := newTestSNMPTopologyCollector()
	var client topologyClientSettings
	coll.newSnmpClient = func(h gosnmp.Handler) snmputils.Session {
		client = topologyClientSettingsOf(h)
		return session
	}
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{{}} }
	var collector ddsnmpcollector.Config
	coll.newDdSnmpColl = func(cfg ddsnmpcollector.Config) ddCollector {
		collector = cfg
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) { return nil, nil })
	}

	_, _, _ = coll.refreshDeviceTopology(context.Background(), testTopologyAttemptID(1), dev, testTopologyTarget())

	assert.Equal(t, topologyClientSettings{
		Target:    "192.0.2.10",
		Port:      161,
		MaxOids:   30,
		Version:   gosnmp.Version2c,
		Community: "fixture-community",
	}, client)
	assert.Equal(t, gosnmp.Version2c, collector.Version)
	assert.Equal(t, 30, collector.MaxOIDs)
	assert.Equal(t, uint32(15), collector.MaxRepetitions)
}

func TestVLANContextConfiguresClientAndProfileCollector(t *testing.T) {
	tests := map[string]struct {
		version         string
		wantCommunity   string
		wantContextName string
	}{
		"SNMPv2c appends the VLAN to the community": {version: "2c", wantCommunity: "fixture-community@100"},
		"SNMPv3 selects the VLAN context":           {version: "3", wantCommunity: "public", wantContextName: "vlan-100"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			dev := ddsnmp.DeviceConnectionInfo{
				Hostname:        "192.0.2.10",
				Port:            161,
				Community:       "fixture-community",
				SNMPVersion:     tc.version,
				V3User:          "fixture-user",
				V3SecurityLevel: "noAuthNoPriv",
				MaxOIDs:         30,
				MaxRepetitions:  15,
			}
			session := snmpmock.NewMockSession(ctrl)
			session.EXPECT().Connect().Return(nil)
			session.EXPECT().Close().Return(nil)

			coll := newTestSNMPTopologyCollector()
			var community, contextName string
			coll.newSnmpClient = func(h gosnmp.Handler) snmputils.Session {
				community, contextName = h.Community(), h.ContextName()
				return session
			}
			var collector ddsnmpcollector.Config
			coll.newDdSnmpColl = func(cfg ddsnmpcollector.Config) ddCollector {
				collector = cfg
				return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) { return nil, nil })
			}

			_, _, err := collectTopologyVLANContext(context.Background(), coll, 1, dev, "100", []*ddsnmp.Profile{{}}, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCommunity, community)
			assert.Equal(t, tc.wantContextName, contextName)
			assert.Equal(t, snmputils.ParseSNMPVersion(tc.version), collector.Version)
			assert.Equal(t, 30, collector.MaxOIDs)
			assert.Equal(t, uint32(15), collector.MaxRepetitions)
		})
	}
}
