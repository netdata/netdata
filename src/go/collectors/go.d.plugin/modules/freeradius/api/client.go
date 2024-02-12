// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"fmt"
	"net"
	"strconv"
	"time"

	"layeh.com/radius"
	"layeh.com/radius/rfc2869"
)

type Status struct {
	AccessRequests        int64 `stm:"access-requests"`
	AccessAccepts         int64 `stm:"access-accepts"`
	AccessRejects         int64 `stm:"access-rejects"`
	AccessChallenges      int64 `stm:"access-challenges"`
	AuthResponses         int64 `stm:"auth-responses"`
	AuthDuplicateRequests int64 `stm:"auth-duplicate-requests"`
	AuthMalformedRequests int64 `stm:"auth-malformed-requests"`
	AuthInvalidRequests   int64 `stm:"auth-invalid-requests"`
	AuthDroppedRequests   int64 `stm:"auth-dropped-requests"`
	AuthUnknownTypes      int64 `stm:"auth-unknown-types"`

	AccountingRequests    int64 `stm:"accounting-requests"`
	AccountingResponses   int64 `stm:"accounting-responses"`
	AcctDuplicateRequests int64 `stm:"acct-duplicate-requests"`
	AcctMalformedRequests int64 `stm:"acct-malformed-requests"`
	AcctInvalidRequests   int64 `stm:"acct-invalid-requests"`
	AcctDroppedRequests   int64 `stm:"acct-dropped-requests"`
	AcctUnknownTypes      int64 `stm:"acct-unknown-types"`

	ProxyAccessRequests        int64 `stm:"proxy-access-requests"`
	ProxyAccessAccepts         int64 `stm:"proxy-access-accepts"`
	ProxyAccessRejects         int64 `stm:"proxy-access-rejects"`
	ProxyAccessChallenges      int64 `stm:"proxy-access-challenges"`
	ProxyAuthResponses         int64 `stm:"proxy-auth-responses"`
	ProxyAuthDuplicateRequests int64 `stm:"proxy-auth-duplicate-requests"`
	ProxyAuthMalformedRequests int64 `stm:"proxy-auth-malformed-requests"`
	ProxyAuthInvalidRequests   int64 `stm:"proxy-auth-invalid-requests"`
	ProxyAuthDroppedRequests   int64 `stm:"proxy-auth-dropped-requests"`
	ProxyAuthUnknownTypes      int64 `stm:"proxy-auth-unknown-types"`

	ProxyAccountingRequests    int64 `stm:"proxy-accounting-requests"`
	ProxyAccountingResponses   int64 `stm:"proxy-accounting-responses"`
	ProxyAcctDuplicateRequests int64 `stm:"proxy-acct-duplicate-requests"`
	ProxyAcctMalformedRequests int64 `stm:"proxy-acct-malformed-requests"`
	ProxyAcctInvalidRequests   int64 `stm:"proxy-acct-invalid-requests"`
	ProxyAcctDroppedRequests   int64 `stm:"proxy-acct-dropped-requests"`
	ProxyAcctUnknownTypes      int64 `stm:"proxy-acct-unknown-types"`
}

type (
	radiusClient interface {
		Exchange(ctx context.Context, packet *radius.Packet, address string) (*radius.Packet, error)
	}
	Config struct {
		Address string
		Port    int
		Secret  string
		Timeout time.Duration
	}
	Client struct {
		address string
		secret  string
		timeout time.Duration
		radiusClient
	}
)

func New(conf Config) *Client {
	return &Client{
		address:      net.JoinHostPort(conf.Address, strconv.Itoa(conf.Port)),
		secret:       conf.Secret,
		timeout:      conf.Timeout,
		radiusClient: &radius.Client{Retry: time.Second, MaxPacketErrors: 10},
	}
}

func (c Client) Status() (*Status, error) {
	packet, err := newStatusServerPacket(c.secret)
	if err != nil {
		return nil, fmt.Errorf("error on creating StatusServer packet: %v", err)
	}

	resp, err := c.queryServer(packet)
	if err != nil {
		return nil, fmt.Errorf("error on request to '%s': %v", c.address, err)
	}

	return decodeResponse(resp), nil
}

func (c Client) queryServer(packet *radius.Packet) (*radius.Packet, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	resp, err := c.Exchange(ctx, packet, c.address)
	if err != nil {
		return nil, err
	}

	if resp.Code != radius.CodeAccessAccept {
		return nil, fmt.Errorf("'%s' returned response code %d", c.address, resp.Code)
	}
	return resp, nil
}

func newStatusServerPacket(secret string) (*radius.Packet, error) {
	// https://wiki.freeradius.org/config/Status#status-of-freeradius-server
	packet := radius.New(radius.CodeStatusServer, []byte(secret))
	if err := FreeRADIUSStatisticsType_Set(packet, FreeRADIUSStatisticsType_Value_All); err != nil {
		return nil, err
	}
	if err := rfc2869.MessageAuthenticator_Set(packet, make([]byte, 16)); err != nil {
		return nil, err
	}
	hash := hmac.New(md5.New, packet.Secret)
	encode, err := packet.Encode()
	if err != nil {
		return nil, err
	}
	if _, err := hash.Write(encode); err != nil {
		return nil, err
	}
	if err := rfc2869.MessageAuthenticator_Set(packet, hash.Sum(nil)); err != nil {
		return nil, err
	}
	return packet, nil
}

func decodeResponse(resp *radius.Packet) *Status {
	return &Status{
		AccessRequests:             int64(FreeRADIUSTotalAccessRequests_Get(resp)),
		AccessAccepts:              int64(FreeRADIUSTotalAccessAccepts_Get(resp)),
		AccessRejects:              int64(FreeRADIUSTotalAccessRejects_Get(resp)),
		AccessChallenges:           int64(FreeRADIUSTotalAccessChallenges_Get(resp)),
		AuthResponses:              int64(FreeRADIUSTotalAuthResponses_Get(resp)),
		AuthDuplicateRequests:      int64(FreeRADIUSTotalAuthDuplicateRequests_Get(resp)),
		AuthMalformedRequests:      int64(FreeRADIUSTotalAuthMalformedRequests_Get(resp)),
		AuthInvalidRequests:        int64(FreeRADIUSTotalAuthInvalidRequests_Get(resp)),
		AuthDroppedRequests:        int64(FreeRADIUSTotalAuthDroppedRequests_Get(resp)),
		AuthUnknownTypes:           int64(FreeRADIUSTotalAuthUnknownTypes_Get(resp)),
		AccountingRequests:         int64(FreeRADIUSTotalAccountingRequests_Get(resp)),
		AccountingResponses:        int64(FreeRADIUSTotalAccountingResponses_Get(resp)),
		AcctDuplicateRequests:      int64(FreeRADIUSTotalAcctDuplicateRequests_Get(resp)),
		AcctMalformedRequests:      int64(FreeRADIUSTotalAcctMalformedRequests_Get(resp)),
		AcctInvalidRequests:        int64(FreeRADIUSTotalAcctInvalidRequests_Get(resp)),
		AcctDroppedRequests:        int64(FreeRADIUSTotalAcctDroppedRequests_Get(resp)),
		AcctUnknownTypes:           int64(FreeRADIUSTotalAcctUnknownTypes_Get(resp)),
		ProxyAccessRequests:        int64(FreeRADIUSTotalProxyAccessRequests_Get(resp)),
		ProxyAccessAccepts:         int64(FreeRADIUSTotalProxyAccessAccepts_Get(resp)),
		ProxyAccessRejects:         int64(FreeRADIUSTotalProxyAccessRejects_Get(resp)),
		ProxyAccessChallenges:      int64(FreeRADIUSTotalProxyAccessChallenges_Get(resp)),
		ProxyAuthResponses:         int64(FreeRADIUSTotalProxyAuthResponses_Get(resp)),
		ProxyAuthDuplicateRequests: int64(FreeRADIUSTotalProxyAuthDuplicateRequests_Get(resp)),
		ProxyAuthMalformedRequests: int64(FreeRADIUSTotalProxyAuthMalformedRequests_Get(resp)),
		ProxyAuthInvalidRequests:   int64(FreeRADIUSTotalProxyAuthInvalidRequests_Get(resp)),
		ProxyAuthDroppedRequests:   int64(FreeRADIUSTotalProxyAuthDroppedRequests_Get(resp)),
		ProxyAuthUnknownTypes:      int64(FreeRADIUSTotalProxyAuthUnknownTypes_Get(resp)),
		ProxyAccountingRequests:    int64(FreeRADIUSTotalProxyAccountingRequests_Get(resp)),
		ProxyAccountingResponses:   int64(FreeRADIUSTotalProxyAccountingResponses_Get(resp)),
		ProxyAcctDuplicateRequests: int64(FreeRADIUSTotalProxyAcctDuplicateRequests_Get(resp)),
		ProxyAcctMalformedRequests: int64(FreeRADIUSTotalProxyAcctMalformedRequests_Get(resp)),
		ProxyAcctInvalidRequests:   int64(FreeRADIUSTotalProxyAcctInvalidRequests_Get(resp)),
		ProxyAcctDroppedRequests:   int64(FreeRADIUSTotalProxyAcctDroppedRequests_Get(resp)),
		ProxyAcctUnknownTypes:      int64(FreeRADIUSTotalProxyAcctUnknownTypes_Get(resp)),
	}
}
