// SPDX-License-Identifier: GPL-3.0-or-later

package freeradius

import (
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/freeradius/api"

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

func TestFreeRADIUS_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &FreeRADIUS{}, dataConfigJSON, dataConfigYAML)
}

func TestFreeRADIUS_Init(t *testing.T) {
	freeRADIUS := New()

	assert.NoError(t, freeRADIUS.Init())
}

func TestFreeRADIUS_Init_ReturnsFalseIfAddressNotSet(t *testing.T) {
	freeRADIUS := New()
	freeRADIUS.Address = ""

	assert.Error(t, freeRADIUS.Init())
}

func TestFreeRADIUS_Init_ReturnsFalseIfPortNotSet(t *testing.T) {
	freeRADIUS := New()
	freeRADIUS.Port = 0

	assert.Error(t, freeRADIUS.Init())
}

func TestFreeRADIUS_Init_ReturnsFalseIfSecretNotSet(t *testing.T) {
	freeRADIUS := New()
	freeRADIUS.Secret = ""

	assert.Error(t, freeRADIUS.Init())
}

func TestFreeRADIUS_Check(t *testing.T) {
	freeRADIUS := New()
	freeRADIUS.client = newOKMockClient()

	assert.NoError(t, freeRADIUS.Check())
}

func TestFreeRADIUS_Check_ReturnsFalseIfClientStatusReturnsError(t *testing.T) {
	freeRADIUS := New()
	freeRADIUS.client = newErrorMockClient()

	assert.Error(t, freeRADIUS.Check())
}

func TestFreeRADIUS_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestFreeRADIUS_Collect(t *testing.T) {
	freeRADIUS := New()
	freeRADIUS.client = newOKMockClient()

	expected := map[string]int64{
		"access-requests":               1,
		"access-accepts":                2,
		"access-rejects":                3,
		"access-challenges":             4,
		"auth-responses":                5,
		"auth-duplicate-requests":       6,
		"auth-malformed-requests":       7,
		"auth-invalid-requests":         8,
		"auth-dropped-requests":         9,
		"auth-unknown-types":            10,
		"accounting-requests":           11,
		"accounting-responses":          12,
		"acct-duplicate-requests":       13,
		"acct-malformed-requests":       14,
		"acct-invalid-requests":         15,
		"acct-dropped-requests":         16,
		"acct-unknown-types":            17,
		"proxy-access-requests":         18,
		"proxy-access-accepts":          19,
		"proxy-access-rejects":          20,
		"proxy-access-challenges":       21,
		"proxy-auth-responses":          22,
		"proxy-auth-duplicate-requests": 23,
		"proxy-auth-malformed-requests": 24,
		"proxy-auth-invalid-requests":   25,
		"proxy-auth-dropped-requests":   26,
		"proxy-auth-unknown-types":      27,
		"proxy-accounting-requests":     28,
		"proxy-accounting-responses":    29,
		"proxy-acct-duplicate-requests": 30,
		"proxy-acct-malformed-requests": 31,
		"proxy-acct-invalid-requests":   32,
		"proxy-acct-dropped-requests":   33,
		"proxy-acct-unknown-types":      34,
	}
	mx := freeRADIUS.Collect()

	assert.Equal(t, expected, mx)
	module.TestMetricsHasAllChartsDims(t, freeRADIUS.Charts(), mx)
}

func TestFreeRADIUS_Collect_ReturnsNilIfClientStatusReturnsError(t *testing.T) {
	freeRADIUS := New()
	freeRADIUS.client = newErrorMockClient()

	assert.Nil(t, freeRADIUS.Collect())
}

func TestFreeRADIUS_Cleanup(t *testing.T) {
	New().Cleanup()
}

func newOKMockClient() *mockClient {
	return &mockClient{}
}

func newErrorMockClient() *mockClient {
	return &mockClient{errOnStatus: true}
}

type mockClient struct {
	errOnStatus bool
}

func (m mockClient) Status() (*api.Status, error) {
	if m.errOnStatus {
		return nil, errors.New("mock Status error")
	}

	status := &api.Status{
		AccessRequests:             1,
		AccessAccepts:              2,
		AccessRejects:              3,
		AccessChallenges:           4,
		AuthResponses:              5,
		AuthDuplicateRequests:      6,
		AuthMalformedRequests:      7,
		AuthInvalidRequests:        8,
		AuthDroppedRequests:        9,
		AuthUnknownTypes:           10,
		AccountingRequests:         11,
		AccountingResponses:        12,
		AcctDuplicateRequests:      13,
		AcctMalformedRequests:      14,
		AcctInvalidRequests:        15,
		AcctDroppedRequests:        16,
		AcctUnknownTypes:           17,
		ProxyAccessRequests:        18,
		ProxyAccessAccepts:         19,
		ProxyAccessRejects:         20,
		ProxyAccessChallenges:      21,
		ProxyAuthResponses:         22,
		ProxyAuthDuplicateRequests: 23,
		ProxyAuthMalformedRequests: 24,
		ProxyAuthInvalidRequests:   25,
		ProxyAuthDroppedRequests:   26,
		ProxyAuthUnknownTypes:      27,
		ProxyAccountingRequests:    28,
		ProxyAccountingResponses:   29,
		ProxyAcctDuplicateRequests: 30,
		ProxyAcctMalformedRequests: 31,
		ProxyAcctInvalidRequests:   32,
		ProxyAcctDroppedRequests:   33,
		ProxyAcctUnknownTypes:      34,
	}
	return status, nil
}
