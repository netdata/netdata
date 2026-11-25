// SPDX-License-Identifier: GPL-3.0-or-later

package wmiquery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/oiweiwei/go-msrpc/dcerpc"
	activation "github.com/oiweiwei/go-msrpc/msrpc/dcom/iactivation/v0"
	"github.com/oiweiwei/go-msrpc/msrpc/erref/hresult"
	"github.com/oiweiwei/go-msrpc/ssp/ntlm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityOptionsUseIndependentNTLMv2Credentials(t *testing.T) {
	ctx := t.Context()
	first := testSecurity(t, "server-a.example.test", "DOMA\\reader", "synthetic-one")
	second := testSecurity(t, "server-b.example.test", "operator@DOMB", "synthetic-two")

	firstNegotiate, err := first.Init(ctx, nil)
	require.NoError(t, err)
	secondNegotiate, err := second.Init(ctx, nil)
	require.NoError(t, err)

	for _, token := range [][]byte{firstNegotiate, secondNegotiate} {
		message := &ntlm.NegotiateMessage{}
		require.NoError(t, message.Unmarshal(ctx, token))
		assert.True(t, message.Negotiate.IsSet(ntlm.NegotiateSign|ntlm.NegotiateSeal|ntlm.Negotiate128))
	}

	challenge := testChallenge(t, ntlm.NegotiateSign|ntlm.NegotiateSeal|ntlm.Negotiate128|ntlm.NegotiateExtendedSessionSecurity|ntlm.NegotiateTargetInfo)
	authFirst, err := first.Init(ctx, challenge)
	require.NoError(t, err)
	authSecond, err := second.Init(ctx, challenge)
	require.NoError(t, err)

	for _, job := range []struct {
		security *dcerpc.Security
		token    []byte
		domain   string
		username string
	}{
		{first, authFirst, "DOMA", "reader"},
		{second, authSecond, "DOMB", "operator"},
	} {
		assert.True(t, job.security.Established())
		message := &ntlm.AuthenticateMessage{}
		require.NoError(t, message.Unmarshal(ctx, job.token))
		assert.Equal(t, job.domain, message.DomainName)
		assert.Equal(t, job.username, message.UserName)
		// NTLMv2 appends the typed client challenge blob after the 16-byte proof.
		require.Greater(t, len(message.NTChallengeResponse), 24)
		assert.Equal(t, []byte{1, 1, 0, 0}, message.NTChallengeResponse[16:20])
	}
}

func TestSecurityDoesNotDowngrade(t *testing.T) {
	for name, flags := range map[string]ntlm.Flag{
		"peer does not seal": ntlm.NegotiateSign | ntlm.Negotiate128 | ntlm.NegotiateTargetInfo,
		"legacy challenge":   ntlm.NegotiateSign | ntlm.Negotiate128 | ntlm.NegotiateSeal,
	} {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			security := testSecurity(t, "server.example.test", "reader", "synthetic-password")
			_, err := security.Init(ctx, nil)
			require.NoError(t, err)

			token, err := security.Init(ctx, testChallenge(t, flags))
			require.Error(t, err)
			assert.Nil(t, token)
			assert.False(t, security.Established())
			assert.Contains(t, SafeMessage(rpcFailure(ctx, "bind", err)), "NTLM")
		})
	}
}

func testSecurity(t *testing.T, host, username, password string) *dcerpc.Security {
	t.Helper()
	opts, err := securityOptions(host, username, password, "ntlm")
	require.NoError(t, err)
	parsed, err := dcerpc.ParseOptions(t.Context(), append(opts, dcerpc.WithAbstractSyntax(activation.ActivationSyntaxV0_0))...)
	require.NoError(t, err)
	require.Equal(t, dcerpc.AuthTypeWinNT, parsed.Security.Type)
	require.Equal(t, dcerpc.AuthLevelPktPrivacy, parsed.Security.Level)
	return parsed.Security
}

func testChallenge(t *testing.T, flags ntlm.Flag) []byte {
	t.Helper()
	message := ntlm.ChallengeMessage{
		Negotiate:       ntlm.NegotiateUnicode | ntlm.NegotiateNTLM | flags,
		ServerChallenge: []byte{0, 1, 2, 3, 4, 5, 6, 7},
	}
	if flags.IsSet(ntlm.NegotiateTargetInfo) {
		message.TargetInfo = ntlm.AttrValues{
			ntlm.AttrNetBIOSComputerName: &ntlm.Value{NetBIOSComputerName: "SERVER"},
		}
	}
	buf, err := message.Marshal(t.Context())
	require.NoError(t, err)
	return buf
}

func TestOpenRejectsMissingSecurityOptions(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	for _, config := range []struct {
		auth     string
		username string
		password string
		want     string
	}{
		{"", "reader", "synthetic", "explicitly set to ntlm"},
		{"kerberos", "reader", "synthetic", "explicitly set to ntlm"},
		{"ntlm", "", "synthetic", "username and password are required"},
		{"ntlm", "DOMAIN\\", "synthetic", "username and password are required"},
		{"ntlm", "reader", "", "username and password are required"},
	} {
		c, err := Open(ctx, "server.example.test", config.username, config.password, config.auth)
		assert.Nil(t, c)
		assert.Contains(t, SafeMessage(err), config.want)
	}
}

func TestScopedIPv6ReachesResolverOnTheSpecifiedInterface(t *testing.T) {
	const host = "fe80::1%test0"
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	var addresses []string
	client := &Client{
		ctx:    ctx,
		cancel: cancel,
		dialer: rpcDialer{
			host: host,
			ctx:  ctx,
			dial: func(_ context.Context, _, address string) (net.Conn, error) {
				addresses = append(addresses, address)
				return nil, syscall.ECONNREFUSED
			},
		},
	}
	defer client.Close()
	err := client.open(host, "reader", "synthetic", "ntlm")
	require.Error(t, err)
	require.NotEmpty(t, addresses)
	for _, address := range addresses {
		assert.Equal(t, "[fe80::1%test0]:135", address)
	}
}

func TestDialerPinsTheConfiguredHostAndClosesOnCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	var addresses []string
	var peers []net.Conn
	dialer := rpcDialer{
		host: "collector-target.example.test",
		ctx:  ctx,
		dial: func(_ context.Context, _, address string) (net.Conn, error) {
			addresses = append(addresses, address)
			client, peer := net.Pipe()
			peers = append(peers, peer)
			t.Cleanup(func() { _ = peer.Close() })
			return fixedRemoteAddr{Conn: client}, nil
		},
	}
	t.Cleanup(dialer.close)

	_, err := dialer.DialContext(ctx, "tcp", "unadvertised.example.test:135")
	require.NoError(t, err)
	_, err = dialer.DialContext(ctx, "tcp", "unadvertised.example.test:49152")
	require.NoError(t, err)
	assert.Equal(t, []string{"collector-target.example.test:135", "192.0.2.1:49152"}, addresses)

	for _, peer := range peers {
		require.NoError(t, peer.SetReadDeadline(time.Now().Add(time.Second)))
	}
	cancel()
	for _, peer := range peers {
		_, err := peer.Read(make([]byte, 1))
		assert.ErrorContains(t, err, "EOF")
	}
}

type fixedRemoteAddr struct{ net.Conn }

func (fixedRemoteAddr) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 135}
}

func TestSafeMessageRedactsRawErrors(t *testing.T) {
	ctx := t.Context()
	raw := fmt.Errorf("untrusted detail: %w", &net.DNSError{Err: "synthetic secret", Name: "private-name"})
	assert.Empty(t, SafeMessage(raw))
	assert.Equal(t, "bind: could not resolve the WMI host", SafeMessage(rpcFailure(ctx, "bind", raw)))
	assert.Equal(t, "bind: connection refused",
		SafeMessage(rpcFailure(ctx, "bind", fmt.Errorf("untrusted detail: %w", syscall.ECONNREFUSED))))
	assert.Equal(t, "open namespace: RPC_E_ACCESS_DENIED (0x8001011b)",
		SafeMessage(rpcFailure(ctx, "open namespace", fmt.Errorf("untrusted detail: %w", hresult.RpcEAccessDenied))))
	rejection := &dcerpc.Result{DefResult: dcerpc.ProviderRejection, ProviderReason: dcerpc.AbstractSyntaxNotSupported}
	assert.Equal(t, "read results: provider_rejection: abstract syntax not supported", SafeMessage(rpcFailure(ctx, "read results", fmt.Errorf("untrusted detail: %w", rejection))))
	assert.Equal(t, "bind: RPC failed", SafeMessage(rpcFailure(ctx, "bind", errors.New("synthetic secret"))))
	assert.Equal(t, "WMI collection timed out", SafeMessage(fmt.Errorf("untrusted detail: %w", context.DeadlineExceeded)))
}
