// SPDX-License-Identifier: GPL-3.0-or-later

package client

import (
	"net/http/httptest"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	_, err := New(web.ClientConfig{}, web.RequestConfig{})
	assert.NoError(t, err)
}

func TestClient_Login(t *testing.T) {
	srv, client := prepareSrvClient(t)
	defer srv.Close()

	assert.NoError(t, client.Login())
	assert.Equal(t, testToken, client.token.get())
}

func TestClient_Logout(t *testing.T) {
	srv, client := prepareSrvClient(t)
	defer srv.Close()

	require.NoError(t, client.Login())

	assert.NoError(t, client.Logout())
	assert.False(t, client.token.isSet())

}

func TestClient_LoggedIn(t *testing.T) {
	srv, client := prepareSrvClient(t)
	defer srv.Close()

	assert.False(t, client.LoggedIn())
	assert.NoError(t, client.Login())
	assert.True(t, client.LoggedIn())
}

func TestClient_APIVersion(t *testing.T) {
	srv, client := prepareSrvClient(t)
	defer srv.Close()

	err := client.Login()
	require.NoError(t, err)

	version, err := client.APIVersion()
	assert.NoError(t, err)
	assert.Equal(t, Version{Major: 2, Minor: 5}, version)
}

func TestClient_Instances(t *testing.T) {
	srv, client := prepareSrvClient(t)
	defer srv.Close()

	err := client.Login()
	require.NoError(t, err)

	instances, err := client.Instances()
	assert.NoError(t, err)
	assert.Equal(t, testInstances, instances)
}

func TestClient_Instances_RetryOnExpiredToken(t *testing.T) {
	srv, client := prepareSrvClient(t)
	defer srv.Close()

	instances, err := client.Instances()
	assert.NoError(t, err)
	assert.Equal(t, testInstances, instances)
}

func TestClient_SelectedStatistics(t *testing.T) {
	srv, client := prepareSrvClient(t)
	defer srv.Close()

	err := client.Login()
	require.NoError(t, err)

	stats, err := client.SelectedStatistics(SelectedStatisticsQuery{})
	assert.NoError(t, err)
	assert.Equal(t, testStatistics, stats)
}

func TestClient_SelectedStatistics_RetryOnExpiredToken(t *testing.T) {
	srv, client := prepareSrvClient(t)
	defer srv.Close()

	stats, err := client.SelectedStatistics(SelectedStatisticsQuery{})
	assert.Equal(t, testStatistics, stats)
	assert.NoError(t, err)
	assert.Equal(t, testStatistics, stats)
}

func prepareSrvClient(t *testing.T) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(MockScaleIOAPIServer{
		User:       testUser,
		Password:   testPassword,
		Version:    testVersion,
		Token:      testToken,
		Instances:  testInstances,
		Statistics: testStatistics,
	})
	client, err := New(web.ClientConfig{}, web.RequestConfig{
		URL:      srv.URL,
		Username: testUser,
		Password: testPassword,
	})
	assert.NoError(t, err)
	return srv, client
}

var (
	testUser      = "user"
	testPassword  = "password"
	testVersion   = "2.5"
	testToken     = "token"
	testInstances = Instances{
		StoragePoolList: []StoragePool{
			{ID: "id1", Name: "Marketing", SparePercentage: 10},
			{ID: "id2", Name: "Finance", SparePercentage: 10},
		},
		SdcList: []Sdc{
			{ID: "id1", SdcIp: "10.0.0.1", MdmConnectionState: "Connected"},
			{ID: "id2", SdcIp: "10.0.0.2", MdmConnectionState: "Connected"},
		},
	}
	testStatistics = SelectedStatistics{
		System:      SystemStatistics{NumOfDevices: 1},
		Sdc:         map[string]SdcStatistics{"id1": {}, "id2": {}},
		StoragePool: map[string]StoragePoolStatistics{"id1": {}, "id2": {}},
	}
)
