// SPDX-License-Identifier: GPL-3.0-or-later

package wmiquery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"

	"github.com/oiweiwei/go-msrpc/dcerpc"
	"github.com/oiweiwei/go-msrpc/msrpc/erref/hresult"
	"github.com/stretchr/testify/assert"
)

func TestSafeMessageRedactsRawErrors(t *testing.T) {
	ctx := t.Context()
	raw := fmt.Errorf("untrusted detail: %w", &net.DNSError{
		Err:  "synthetic secret",
		Name: "private-name",
	})
	assert.Empty(t, SafeMessage(raw))
	assert.Equal(t, "bind: could not resolve the WMI host", SafeMessage(rpcFailure(ctx, "NTLM", "bind", raw)))
	assert.Equal(t, "bind: connection refused",
		SafeMessage(rpcFailure(ctx, "NTLM", "bind", fmt.Errorf("untrusted detail: %w", syscall.ECONNREFUSED))))
	assert.Equal(
		t,
		"open namespace: RPC_E_ACCESS_DENIED (0x8001011b)",
		SafeMessage(
			rpcFailure(ctx, "NTLM", "open namespace", fmt.Errorf("untrusted detail: %w", hresult.RpcEAccessDenied)),
		),
	)
	rejection := &dcerpc.Result{
		DefResult:      dcerpc.ProviderRejection,
		ProviderReason: dcerpc.AbstractSyntaxNotSupported,
	}
	assert.Equal(
		t,
		"read results: provider_rejection: abstract syntax not supported",
		SafeMessage(rpcFailure(ctx, "NTLM", "read results", fmt.Errorf("untrusted detail: %w", rejection))),
	)
	assert.Equal(t, "bind: RPC failed", SafeMessage(rpcFailure(ctx, "NTLM", "bind", errors.New("synthetic secret"))))
	assert.Equal(
		t,
		"WMI collection timed out",
		SafeMessage(fmt.Errorf("untrusted detail: %w", context.DeadlineExceeded)),
	)
}
