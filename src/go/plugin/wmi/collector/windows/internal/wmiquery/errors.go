// SPDX-License-Identifier: GPL-3.0-or-later

package wmiquery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"

	rpcerrors "github.com/oiweiwei/go-msrpc/dcerpc/errors"
	"github.com/oiweiwei/go-msrpc/msrpc/erref/hresult"
	"github.com/oiweiwei/go-msrpc/msrpc/erref/win32"
	wmistatus "github.com/oiweiwei/go-msrpc/msrpc/erref/wmi"
	"github.com/oiweiwei/go-msrpc/ssp/gssapi"
	"github.com/oiweiwei/go-msrpc/ssp/ntlm"
)

type diagnostic string

func (d diagnostic) Error() string { return string(d) }

// SafeMessage returns a diagnostic containing only fixed text and protocol status
// codes. It returns an empty string for errors outside this package.
func SafeMessage(err error) string {
	var safe diagnostic
	if errors.As(err, &safe) {
		return string(safe)
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "WMI collection canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "WMI collection timed out"
	default:
		return ""
	}
}

func statusError(operation string, code uint32) error {
	for _, known := range []error{wmistatus.FromCode(int32(code)), hresult.FromCode(code), rpcerrors.FromCode(code), win32.FromCode(code)} {
		switch status := known.(type) {
		case *wmistatus.Error:
			return diagnostic(fmt.Sprintf("%s: %s (0x%08x)", operation, status.Name, code))
		case *hresult.Error:
			return diagnostic(fmt.Sprintf("%s: %s (0x%08x)", operation, status.Name, code))
		case *rpcerrors.RPCError:
			return diagnostic(fmt.Sprintf("%s: %s (0x%08x)", operation, status.Name, code))
		case *win32.Error:
			return diagnostic(fmt.Sprintf("%s: %s (0x%08x)", operation, status.Name, code))
		}
	}
	return diagnostic(fmt.Sprintf("%s: status 0x%08x", operation, code))
}

// RPC/SSP error chains can contain principal names and server-supplied text.
func rpcFailure(ctx context.Context, operation string, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%s: %w", operation, ctx.Err())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: %w", operation, context.DeadlineExceeded)
	}
	var (
		result  *hresult.Error
		wmiCode *wmistatus.Error
		rpcCode *rpcerrors.RPCError
		winCode *win32.Error
		rawCode *rpcerrors.Error
		dnsErr  *net.DNSError
		netErr  net.Error
		authErr *gssapi.Error
	)
	switch {
	case errors.As(err, &result):
		return statusError(operation, result.Code)
	case errors.As(err, &wmiCode):
		return statusError(operation, wmiCode.Code)
	case errors.As(err, &rpcCode):
		return statusError(operation, rpcCode.Code)
	case errors.As(err, &winCode):
		return statusError(operation, winCode.Code)
	case errors.As(err, &rawCode):
		switch code := rawCode.Value.(type) {
		case int32:
			return statusError(operation, uint32(code))
		case uint32:
			return statusError(operation, code)
		}
	case errors.As(err, &netErr) && netErr.Timeout():
		return diagnostic(operation + ": network request timed out")
	case errors.As(err, &dnsErr):
		return diagnostic(operation + ": could not resolve the WMI host")
	case errors.Is(err, syscall.ECONNREFUSED), errors.Is(err, syscall.Errno(10061)): // WSAECONNREFUSED
		return diagnostic(operation + ": connection refused")
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH),
		errors.Is(err, syscall.Errno(10065)), errors.Is(err, syscall.Errno(10051)): // WSAEHOSTUNREACH, WSAENETUNREACH
		return diagnostic(operation + ": host or network unreachable")
	case errors.Is(err, ntlm.ErrConfidentialityNotAvailable), errors.Is(err, ntlm.Err128BitEncryptionNotSupported):
		return diagnostic(operation + ": server did not negotiate NTLM packet privacy")
	case errors.Is(err, ntlm.ErrLogonFailure), errors.As(err, &authErr):
		return diagnostic(operation + ": NTLM authentication failed")
	}
	return diagnostic(operation + ": RPC failed")
}
