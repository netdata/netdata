// SPDX-License-Identifier: GPL-3.0-or-later

package vcsa

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

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

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	collr := prepareVCSA()

	assert.NoError(t, collr.Init(context.Background()))
	assert.NotNil(t, collr.client)
}

func TestCollector_InitErrorOnValidatingInitParameters(t *testing.T) {
	collr := New()

	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_InitErrorOnCreatingClient(t *testing.T) {
	collr := prepareVCSA()
	collr.ClientConfig.TLSConfig.TLSCA = "testdata/tls"

	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_Check(t *testing.T) {
	collr := prepareVCSA()
	require.NoError(t, collr.Init(context.Background()))
	collr.client = &mockVCenterHealthClient{}

	assert.NoError(t, collr.Check(context.Background()))
}

func TestCollector_CheckErrorOnLogin(t *testing.T) {
	collr := prepareVCSA()
	require.NoError(t, collr.Init(context.Background()))
	collr.client = &mockVCenterHealthClient{
		login: func() error { return errors.New("login mock error") },
	}

	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_CheckEnsureLoggedIn(t *testing.T) {
	collr := prepareVCSA()
	require.NoError(t, collr.Init(context.Background()))
	mock := &mockVCenterHealthClient{}
	collr.client = mock

	assert.NoError(t, collr.Check(context.Background()))
	assert.True(t, mock.loginCalls == 1)
}

func TestCollector_Cleanup(t *testing.T) {
	collr := prepareVCSA()
	require.NoError(t, collr.Init(context.Background()))
	mock := &mockVCenterHealthClient{}
	collr.client = mock
	collr.Cleanup(context.Background())

	assert.True(t, mock.logoutCalls == 1)
}

func TestCollector_CleanupWithNilClient(t *testing.T) {
	collr := prepareVCSA()

	assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Collect(t *testing.T) {
	collr := prepareVCSA()
	require.NoError(t, collr.Init(context.Background()))
	mock := &mockVCenterHealthClient{}
	collr.client = mock

	expected := map[string]int64{
		"applmgmt_status_gray":             0,
		"applmgmt_status_green":            1,
		"applmgmt_status_orange":           0,
		"applmgmt_status_red":              0,
		"applmgmt_status_unknown":          0,
		"applmgmt_status_yellow":           0,
		"database_storage_status_gray":     0,
		"database_storage_status_green":    1,
		"database_storage_status_orange":   0,
		"database_storage_status_red":      0,
		"database_storage_status_unknown":  0,
		"database_storage_status_yellow":   0,
		"load_status_gray":                 0,
		"load_status_green":                1,
		"load_status_orange":               0,
		"load_status_red":                  0,
		"load_status_unknown":              0,
		"load_status_yellow":               0,
		"mem_status_gray":                  0,
		"mem_status_green":                 1,
		"mem_status_orange":                0,
		"mem_status_red":                   0,
		"mem_status_unknown":               0,
		"mem_status_yellow":                0,
		"software_packages_status_gray":    0,
		"software_packages_status_green":   1,
		"software_packages_status_orange":  0,
		"software_packages_status_red":     0,
		"software_packages_status_unknown": 0,
		"storage_status_gray":              0,
		"storage_status_green":             1,
		"storage_status_orange":            0,
		"storage_status_red":               0,
		"storage_status_unknown":           0,
		"storage_status_yellow":            0,
		"swap_status_gray":                 0,
		"swap_status_green":                1,
		"swap_status_orange":               0,
		"swap_status_red":                  0,
		"swap_status_unknown":              0,
		"swap_status_yellow":               0,
		"system_status_gray":               0,
		"system_status_green":              1,
		"system_status_orange":             0,
		"system_status_red":                0,
		"system_status_unknown":            0,
		"system_status_yellow":             0,
	}

	assert.Equal(t, expected, collr.Collect(context.Background()))
}

func TestCollector_CollectEnsurePingIsCalled(t *testing.T) {
	collr := prepareVCSA()
	require.NoError(t, collr.Init(context.Background()))
	mock := &mockVCenterHealthClient{}
	collr.client = mock
	collr.Collect(context.Background())

	assert.True(t, mock.pingCalls == 1)
}

func TestCollector_CollectErrorOnPing(t *testing.T) {
	collr := prepareVCSA()
	require.NoError(t, collr.Init(context.Background()))
	mock := &mockVCenterHealthClient{
		ping: func() error { return errors.New("ping mock error") },
	}
	collr.client = mock

	assert.Zero(t, collr.Collect(context.Background()))
}

func TestCollector_CollectErrorOnHealthCalls(t *testing.T) {
	collr := prepareVCSA()
	require.NoError(t, collr.Init(context.Background()))
	mock := &mockVCenterHealthClient{
		applMgmt:         func() (string, error) { return "", errors.New("applMgmt mock error") },
		databaseStorage:  func() (string, error) { return "", errors.New("databaseStorage mock error") },
		load:             func() (string, error) { return "", errors.New("load mock error") },
		mem:              func() (string, error) { return "", errors.New("mem mock error") },
		softwarePackages: func() (string, error) { return "", errors.New("softwarePackages mock error") },
		storage:          func() (string, error) { return "", errors.New("storage mock error") },
		swap:             func() (string, error) { return "", errors.New("swap mock error") },
		system:           func() (string, error) { return "", errors.New("system mock error") },
	}
	collr.client = mock

	assert.Zero(t, collr.Collect(context.Background()))
}

func prepareVCSA() *Collector {
	vc := New()
	vc.URL = "https://127.0.0.1:38001"
	vc.Username = "user"
	vc.Password = "pass"

	return vc
}

type mockVCenterHealthClient struct {
	login            func() error
	logout           func() error
	ping             func() error
	applMgmt         func() (string, error)
	databaseStorage  func() (string, error)
	load             func() (string, error)
	mem              func() (string, error)
	softwarePackages func() (string, error)
	storage          func() (string, error)
	swap             func() (string, error)
	system           func() (string, error)
	loginCalls       int
	logoutCalls      int
	pingCalls        int
}

func (m *mockVCenterHealthClient) Login() error {
	m.loginCalls += 1
	if m.login == nil {
		return nil
	}
	return m.login()
}

func (m *mockVCenterHealthClient) Logout() error {
	m.logoutCalls += 1
	if m.logout == nil {
		return nil
	}
	return m.logout()
}

func (m *mockVCenterHealthClient) Ping() error {
	m.pingCalls += 1
	if m.ping == nil {
		return nil
	}
	return m.ping()
}

func (m *mockVCenterHealthClient) ApplMgmt() (string, error) {
	if m.applMgmt == nil {
		return "green", nil
	}
	return m.applMgmt()
}

func (m *mockVCenterHealthClient) DatabaseStorage() (string, error) {
	if m.databaseStorage == nil {
		return "green", nil
	}
	return m.databaseStorage()
}

func (m *mockVCenterHealthClient) Load() (string, error) {
	if m.load == nil {
		return "green", nil
	}
	return m.load()
}

func (m *mockVCenterHealthClient) Mem() (string, error) {
	if m.mem == nil {
		return "green", nil
	}
	return m.mem()
}

func (m *mockVCenterHealthClient) SoftwarePackages() (string, error) {
	if m.softwarePackages == nil {
		return "green", nil
	}
	return m.softwarePackages()
}

func (m *mockVCenterHealthClient) Storage() (string, error) {
	if m.storage == nil {
		return "green", nil
	}
	return m.storage()
}

func (m *mockVCenterHealthClient) Swap() (string, error) {
	if m.swap == nil {
		return "green", nil
	}
	return m.swap()
}

func (m *mockVCenterHealthClient) System() (string, error) {
	if m.system == nil {
		return "green", nil
	}
	return m.system()
}
