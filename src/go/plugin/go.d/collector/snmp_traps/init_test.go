// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"net"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorChartTemplateYAML(t *testing.T) {
	collecttest.AssertChartTemplateSchema(t, New().ChartTemplateYAML())
}

func TestValidateJobName(t *testing.T) {
	tests := map[string]struct {
		name    string
		wantErr bool
	}{
		"valid simple":            {name: "local"},
		"valid with underscore":   {name: "my_job"},
		"valid with dash":         {name: "my-job"},
		"valid with numbers":      {name: "job123"},
		"valid alphanumeric":      {name: "a1_b2-c3"},
		"valid single char":       {name: "a"},
		"valid 64 chars":          {name: "a123456789012345678901234567890123456789012345678901234567890123"},
		"empty":                   {name: "", wantErr: true},
		"too long 65 chars":       {name: "a1234567890123456789012345678901234567890123456789012345678901234", wantErr: true},
		"contains dot":            {name: "my.job", wantErr: true},
		"contains slash":          {name: "my/job", wantErr: true},
		"contains backslash":      {name: "my\\job", wantErr: true},
		"contains control char":   {name: "my\x00job", wantErr: true},
		"contains colon":          {name: "my:job", wantErr: true},
		"contains space":          {name: "my job", wantErr: true},
		"starts with dash":        {name: "-job", wantErr: true},
		"starts with underscore":  {name: "_job", wantErr: true},
		"valid starts with digit": {name: "1job"},
	}

	for tcName, tc := range tests {
		t.Run(tcName, func(t *testing.T) {
			err := validateJobName(tc.name)
			if tc.wantErr {
				assert.Error(t, err, "validateJobName(%q) should fail", tc.name)
			} else {
				assert.NoError(t, err, "validateJobName(%q) should pass", tc.name)
			}
		})
	}
}

func TestValidateEndpoints(t *testing.T) {
	tests := map[string]struct {
		endpoints []EndpointConfig
		wantErr   bool
		errMsg    string
	}{
		"valid single endpoint": {
			endpoints: []EndpointConfig{{Protocol: "udp", Address: "0.0.0.0", Port: 162}},
		},
		"valid IPv6 endpoint": {
			endpoints: []EndpointConfig{{Protocol: "udp", Address: "::1", Port: 162}},
		},
		"valid multiple endpoints": {
			endpoints: []EndpointConfig{
				{Protocol: "udp", Address: "0.0.0.0", Port: 162},
				{Protocol: "udp", Address: "::1", Port: 3162},
			},
		},
		"duplicate endpoint": {
			endpoints: []EndpointConfig{
				{Protocol: "udp", Address: "127.0.0.1", Port: 162},
				{Protocol: "UDP", Address: "127.0.0.1", Port: 162},
			},
			wantErr: true, errMsg: "duplicate endpoint",
		},
		"empty endpoints": {
			endpoints: nil, wantErr: true, errMsg: "at least one endpoint",
		},
		"unsupported protocol": {
			endpoints: []EndpointConfig{{Protocol: "tcp", Address: "0.0.0.0", Port: 162}},
			wantErr:   true, errMsg: "unsupported protocol",
		},
		"missing address": {
			endpoints: []EndpointConfig{{Protocol: "udp", Address: "", Port: 162}},
			wantErr:   true, errMsg: "address is required",
		},
		"invalid port zero": {
			endpoints: []EndpointConfig{{Protocol: "udp", Address: "0.0.0.0", Port: 0}},
			wantErr:   true, errMsg: "port must be",
		},
		"invalid port too high": {
			endpoints: []EndpointConfig{{Protocol: "udp", Address: "0.0.0.0", Port: 65536}},
			wantErr:   true, errMsg: "port must be",
		},
		"invalid address": {
			endpoints: []EndpointConfig{{Protocol: "udp", Address: "not-an-address", Port: 162}},
			wantErr:   true, errMsg: "invalid address/port",
		},
	}

	for tcName, tc := range tests {
		t.Run(tcName, func(t *testing.T) {
			err := validateEndpoints(tc.endpoints)
			if tc.wantErr {
				assert.Error(t, err)
				if tc.errMsg != "" {
					assert.Contains(t, err.Error(), tc.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateVersions(t *testing.T) {
	tests := map[string]struct {
		versions []string
		want     []string
		wantErr  bool
		errMsg   string
	}{
		"valid v1": {
			versions: []string{"v1"},
			want:     []string{"v1"},
		},
		"valid v2c": {
			versions: []string{"v2c"},
			want:     []string{"v2c"},
		},
		"valid both normalized": {
			versions: []string{" V1 ", "V2C"},
			want:     []string{"v1", "v2c"},
		},
		"empty": {
			versions: nil, wantErr: true, errMsg: "at least one SNMP version",
		},
		"valid v3": {
			versions: []string{"v3"},
			want:     []string{"v3"},
		},
		"duplicate normalized": {
			versions: []string{"v2c", "V2C"}, wantErr: true, errMsg: "duplicate SNMP version",
		},
	}

	for tcName, tc := range tests {
		t.Run(tcName, func(t *testing.T) {
			got, err := validateVersions(tc.versions)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCollectorInit_BindsEndpointsAndCheckIsNoop(t *testing.T) {
	setMinimalProfileDir(t)
	port := freeUDPPort(t)

	c := New()
	c.SetJobName("local")
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: port}}

	require.NoError(t, c.Init(context.Background()))
	require.NotNil(t, c.listener)
	require.NoError(t, c.Check(context.Background()))

	c.Cleanup(context.Background())
	require.Nil(t, c.listener)
}

func TestCollectorInit_IdempotentDoubleInit(t *testing.T) {
	setMinimalProfileDir(t)
	port := freeUDPPort(t)

	c := New()
	c.SetJobName("local")
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: port}}

	require.NoError(t, c.Init(context.Background()))
	require.NotNil(t, c.listener)
	first := c.listener
	require.NoError(t, c.Init(context.Background()))
	assert.Same(t, first, c.listener)

	c.Cleanup(context.Background())
	require.Nil(t, c.listener)
}

func TestCollectorInit_InvalidJobNameIsCodedError(t *testing.T) {
	c := New()
	c.SetJobName("../bad")
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: 162}}

	err := c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ Code() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 422, coded.Code())
	assert.Nil(t, c.listener)
}

func TestCollectorInit_InvalidEndpointsIsCodedError(t *testing.T) {
	c := New()
	c.SetJobName("local")
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "tcp", Address: "127.0.0.1", Port: 162}}

	err := c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ Code() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 422, coded.Code())
	assert.Nil(t, c.listener)
}

func TestCollectorInit_BindsMultipleEndpoints(t *testing.T) {
	setMinimalProfileDir(t)
	firstPort := freeUDPPort(t)
	secondPort := freeUDPPort(t)

	c := New()
	c.SetJobName("local")
	c.Listen.Endpoints = []EndpointConfig{
		{Protocol: "udp", Address: "127.0.0.1", Port: firstPort},
		{Protocol: "udp", Address: "127.0.0.1", Port: secondPort},
	}

	require.NoError(t, c.Init(context.Background()))
	require.NotNil(t, c.listener)
	require.Len(t, c.listener.endpoints, 2)

	c.Cleanup(context.Background())
	require.Nil(t, c.listener)

	firstConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: firstPort})
	require.NoError(t, err, "first endpoint should close on cleanup")
	require.NoError(t, firstConn.Close())

	secondConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: secondPort})
	require.NoError(t, err, "second endpoint should close on cleanup")
	require.NoError(t, secondConn.Close())
}

func TestCollectorInit_BindFailureIsCodedError(t *testing.T) {
	setMinimalProfileDir(t)
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	defer conn.Close()

	port := conn.LocalAddr().(*net.UDPAddr).Port

	c := New()
	c.SetJobName("local")
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: port}}

	err = c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ Code() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 422, coded.Code())
	assert.Nil(t, c.listener)
}

func TestCollectorInit_InvalidVersionIsCodedError(t *testing.T) {
	c := New()
	c.SetJobName("local")
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: 162}}
	c.Versions = []string{"v5"}

	err := c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ Code() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 422, coded.Code())
	assert.Nil(t, c.listener)
}

func TestCollectorInit_ProfileLoadFailureIsCodedError(t *testing.T) {
	setTestDirs(t, t.TempDir())
	resetProfileCacheForTest()

	c := New()
	c.SetJobName("local")
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: freeUDPPort(t)}}
	c.Versions = []string{" V1 ", "V2C"}

	err := c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ Code() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 422, coded.Code())
	assert.Nil(t, c.listener)
	assert.Equal(t, []string{" V1 ", "V2C"}, c.Versions)
}

func TestCollectorInit_PartialBindFailureClosesPriorSockets(t *testing.T) {
	setMinimalProfileDir(t)
	firstPort := freeUDPPort(t)
	secondConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	defer secondConn.Close()
	secondPort := secondConn.LocalAddr().(*net.UDPAddr).Port

	c := New()
	c.SetJobName("local")
	c.Listen.Endpoints = []EndpointConfig{
		{Protocol: "udp", Address: "127.0.0.1", Port: firstPort},
		{Protocol: "udp", Address: "127.0.0.1", Port: secondPort},
	}

	err = c.Init(context.Background())
	require.Error(t, err)
	assert.Nil(t, c.listener)

	firstConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: firstPort})
	require.NoError(t, err, "first endpoint should have been closed after partial bind failure")
	require.NoError(t, firstConn.Close())
}

func TestCollectorInit_CleansCreatedV3StateOnEngineBootsFailure(t *testing.T) {
	setMinimalProfileDir(t)
	withEngineStateDir(t)

	const jobName = "cleanup-v3-state"
	require.NoError(t, os.MkdirAll(engineBootsPath(jobName), 0750))

	c := New()
	c.SetJobName(jobName)
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: freeUDPPort(t)}}
	c.Versions = []string{"v3"}
	c.USMUsers = []USMUserConfig{{
		Username:  "testuser",
		EngineID:  testEngineIDHex,
		AuthProto: "sha256",
		AuthKey:   "authpassword",
		PrivProto: "aes",
		PrivKey:   "privpassword",
	}}
	c.EngineIDWhitelist = []string{testEngineIDHex}

	err := c.Init(context.Background())
	require.Error(t, err)
	assert.NoFileExists(t, localEngineIDPath(jobName))
	assert.DirExists(t, engineBootsPath(jobName), "pre-existing state path must not be removed")
	assert.Nil(t, c.listener)
}

func TestCollectorCleanupIsIdempotent(t *testing.T) {
	setMinimalProfileDir(t)
	port := freeUDPPort(t)

	c := New()
	c.SetJobName("local")
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: port}}

	require.NoError(t, c.Init(context.Background()))
	require.NotNil(t, c.listener)

	c.Cleanup(context.Background())
	c.Cleanup(context.Background())
	require.Nil(t, c.listener)
}

func TestCollectorCollectRequiresStartedListener(t *testing.T) {
	c := New()
	err := c.Collect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listener not started")
}

func freeUDPPort(t *testing.T) int {
	t.Helper()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	defer conn.Close()

	return conn.LocalAddr().(*net.UDPAddr).Port
}
