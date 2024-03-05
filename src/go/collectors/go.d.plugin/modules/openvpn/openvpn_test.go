// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn

import (
	"os"
	"testing"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/modules/openvpn/client"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/matcher"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/socket"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestOpenVPN_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &OpenVPN{}, dataConfigJSON, dataConfigYAML)
}

func TestOpenVPN_Init(t *testing.T) {
	assert.NoError(t, New().Init())
}

func TestOpenVPN_Check(t *testing.T) {
	job := New()

	require.NoError(t, job.Init())
	job.client = prepareMockOpenVPNClient()
	require.NoError(t, job.Check())
}

func TestOpenVPN_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestOpenVPN_Cleanup(t *testing.T) {
	job := New()

	assert.NotPanics(t, job.Cleanup)
	require.NoError(t, job.Init())
	job.client = prepareMockOpenVPNClient()
	require.NoError(t, job.Check())
	job.Cleanup()
}

func TestOpenVPN_Collect(t *testing.T) {
	job := New()

	require.NoError(t, job.Init())
	job.perUserMatcher = matcher.TRUE()
	job.client = prepareMockOpenVPNClient()
	require.NoError(t, job.Check())

	expected := map[string]int64{
		"bytes_in":            1,
		"bytes_out":           1,
		"clients":             1,
		"name_bytes_received": 1,
		"name_bytes_sent":     2,
	}

	mx := job.Collect()
	require.NotNil(t, mx)
	delete(mx, "name_connection_time")
	assert.Equal(t, expected, mx)
}

func TestOpenVPN_Collect_UNDEFUsername(t *testing.T) {
	job := New()

	require.NoError(t, job.Init())
	job.perUserMatcher = matcher.TRUE()
	cl := prepareMockOpenVPNClient()
	cl.users = testUsersUNDEF
	job.client = cl
	require.NoError(t, job.Check())

	expected := map[string]int64{
		"bytes_in":                   1,
		"bytes_out":                  1,
		"clients":                    1,
		"common_name_bytes_received": 1,
		"common_name_bytes_sent":     2,
	}

	mx := job.Collect()
	require.NotNil(t, mx)
	delete(mx, "common_name_connection_time")
	assert.Equal(t, expected, mx)
}

func prepareMockOpenVPNClient() *mockOpenVPNClient {
	return &mockOpenVPNClient{
		version:   testVersion,
		loadStats: testLoadStats,
		users:     testUsers,
	}
}

type mockOpenVPNClient struct {
	version   client.Version
	loadStats client.LoadStats
	users     client.Users
}

func (m *mockOpenVPNClient) Connect() error                        { return nil }
func (m *mockOpenVPNClient) Disconnect() error                     { return nil }
func (m *mockOpenVPNClient) Version() (*client.Version, error)     { return &m.version, nil }
func (m *mockOpenVPNClient) LoadStats() (*client.LoadStats, error) { return &m.loadStats, nil }
func (m *mockOpenVPNClient) Users() (client.Users, error)          { return m.users, nil }
func (m *mockOpenVPNClient) Command(_ string, _ socket.Processor) error {
	// mocks are done on the individual commands. e.g. in Version() below
	panic("should be called in the mock")
}

var (
	testVersion   = client.Version{Major: 1, Minor: 1, Patch: 1, Management: 1}
	testLoadStats = client.LoadStats{NumOfClients: 1, BytesIn: 1, BytesOut: 1}
	testUsers     = client.Users{{
		CommonName:     "common_name",
		RealAddress:    "1.2.3.4:4321",
		VirtualAddress: "1.2.3.4",
		BytesReceived:  1,
		BytesSent:      2,
		ConnectedSince: 3,
		Username:       "name",
	}}
	testUsersUNDEF = client.Users{{
		CommonName:     "common_name",
		RealAddress:    "1.2.3.4:4321",
		VirtualAddress: "1.2.3.4",
		BytesReceived:  1,
		BytesSent:      2,
		ConnectedSince: 3,
		Username:       "UNDEF",
	}}
)
