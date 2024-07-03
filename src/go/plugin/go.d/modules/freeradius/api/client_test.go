// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"layeh.com/radius"
)

func TestNew(t *testing.T) {
	assert.NotNil(t, New(Config{}))
}

func TestClient_Status(t *testing.T) {
	var c Client
	c.radiusClient = newOKMockFreeRADIUSClient()

	expected := Status{
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

	s, err := c.Status()

	require.NoError(t, err)
	assert.Equal(t, expected, *s)
}

func TestClient_Status_ReturnsErrorIfClientExchangeReturnsError(t *testing.T) {
	var c Client
	c.radiusClient = newErrorMockFreeRADIUSClient()

	s, err := c.Status()

	assert.Nil(t, s)
	assert.Error(t, err)
}

func TestClient_Status_ReturnsErrorIfServerResponseHasBadStatus(t *testing.T) {
	var c Client
	c.radiusClient = newBadRespCodeMockFreeRADIUSClient()

	s, err := c.Status()

	assert.Nil(t, s)
	assert.Error(t, err)
}

type mockFreeRADIUSClient struct {
	errOnExchange bool
	badRespCode   bool
}

func newOKMockFreeRADIUSClient() *mockFreeRADIUSClient {
	return &mockFreeRADIUSClient{}
}

func newErrorMockFreeRADIUSClient() *mockFreeRADIUSClient {
	return &mockFreeRADIUSClient{errOnExchange: true}
}

func newBadRespCodeMockFreeRADIUSClient() *mockFreeRADIUSClient {
	return &mockFreeRADIUSClient{badRespCode: true}
}

func (m mockFreeRADIUSClient) Exchange(_ context.Context, _ *radius.Packet, _ string) (*radius.Packet, error) {
	if m.errOnExchange {
		return nil, errors.New("mock Exchange error")
	}
	resp := radius.New(radius.CodeAccessAccept, []byte("secret"))
	if m.badRespCode {
		resp.Code = radius.CodeAccessReject
	} else {
		resp.Code = radius.CodeAccessAccept
	}
	addValues(resp)
	return resp, nil
}

func addValues(resp *radius.Packet) {
	_ = FreeRADIUSTotalAccessRequests_Add(resp, 1)
	_ = FreeRADIUSTotalAccessAccepts_Add(resp, 2)
	_ = FreeRADIUSTotalAccessRejects_Add(resp, 3)
	_ = FreeRADIUSTotalAccessChallenges_Add(resp, 4)
	_ = FreeRADIUSTotalAuthResponses_Add(resp, 5)
	_ = FreeRADIUSTotalAuthDuplicateRequests_Add(resp, 6)
	_ = FreeRADIUSTotalAuthMalformedRequests_Add(resp, 7)
	_ = FreeRADIUSTotalAuthInvalidRequests_Add(resp, 8)
	_ = FreeRADIUSTotalAuthDroppedRequests_Add(resp, 9)
	_ = FreeRADIUSTotalAuthUnknownTypes_Add(resp, 10)
	_ = FreeRADIUSTotalAccountingRequests_Add(resp, 11)
	_ = FreeRADIUSTotalAccountingResponses_Add(resp, 12)
	_ = FreeRADIUSTotalAcctDuplicateRequests_Add(resp, 13)
	_ = FreeRADIUSTotalAcctMalformedRequests_Add(resp, 14)
	_ = FreeRADIUSTotalAcctInvalidRequests_Add(resp, 15)
	_ = FreeRADIUSTotalAcctDroppedRequests_Add(resp, 16)
	_ = FreeRADIUSTotalAcctUnknownTypes_Add(resp, 17)
	_ = FreeRADIUSTotalProxyAccessRequests_Add(resp, 18)
	_ = FreeRADIUSTotalProxyAccessAccepts_Add(resp, 19)
	_ = FreeRADIUSTotalProxyAccessRejects_Add(resp, 20)
	_ = FreeRADIUSTotalProxyAccessChallenges_Add(resp, 21)
	_ = FreeRADIUSTotalProxyAuthResponses_Add(resp, 22)
	_ = FreeRADIUSTotalProxyAuthDuplicateRequests_Add(resp, 23)
	_ = FreeRADIUSTotalProxyAuthMalformedRequests_Add(resp, 24)
	_ = FreeRADIUSTotalProxyAuthInvalidRequests_Add(resp, 25)
	_ = FreeRADIUSTotalProxyAuthDroppedRequests_Add(resp, 26)
	_ = FreeRADIUSTotalProxyAuthUnknownTypes_Add(resp, 27)
	_ = FreeRADIUSTotalProxyAccountingRequests_Add(resp, 28)
	_ = FreeRADIUSTotalProxyAccountingResponses_Add(resp, 29)
	_ = FreeRADIUSTotalProxyAcctDuplicateRequests_Add(resp, 30)
	_ = FreeRADIUSTotalProxyAcctMalformedRequests_Add(resp, 31)
	_ = FreeRADIUSTotalProxyAcctInvalidRequests_Add(resp, 32)
	_ = FreeRADIUSTotalProxyAcctDroppedRequests_Add(resp, 33)
	_ = FreeRADIUSTotalProxyAcctUnknownTypes_Add(resp, 34)
}
