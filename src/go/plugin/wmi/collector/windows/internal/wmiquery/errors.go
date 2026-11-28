// SPDX-License-Identifier: GPL-3.0-or-later

package wmiquery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"

	"github.com/oiweiwei/go-msrpc/dcerpc"
	rpcerrors "github.com/oiweiwei/go-msrpc/dcerpc/errors"
	"github.com/oiweiwei/go-msrpc/msrpc/erref/hresult"
	"github.com/oiweiwei/go-msrpc/msrpc/erref/win32"
	wmistatus "github.com/oiweiwei/go-msrpc/msrpc/erref/wmi"
	"github.com/oiweiwei/go-msrpc/ssp/gssapi"
	"github.com/oiweiwei/go-msrpc/ssp/ntlm"
)

type diagnostic string

func (d diagnostic) Error() string { return string(d) }

// sessionError marks a failure that makes the whole session unusable: a refused
// connection, an untrusted certificate or a rejected login.
type sessionError struct{ error }

func (e sessionError) Unwrap() error { return e.error }

func (sessionError) SessionFailed() bool { return true }

// SessionFailed reports whether err made the session unusable, so every later
// query in it fails the same way without contacting the host.
func SessionFailed(err error) bool {
	var failed interface{ SessionFailed() bool }
	return errors.As(err, &failed) && failed.SessionFailed()
}

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

// statusError names a WMI, HRESULT, RPC or Win32 status code, in that order of precedence.
func statusError(operation string, code uint32) error {
	if name := statusName(code); name != "" {
		return diagnostic(fmt.Sprintf("%s: %s (0x%08x)", operation, name, code))
	}
	return diagnostic(fmt.Sprintf("%s: status 0x%08x", operation, code))
}

func statusName(code uint32) string {
	for _, known := range []error{wmistatus.FromCode(int32(code)), hresult.FromCode(code), rpcerrors.FromCode(code), win32.FromCode(code)} {
		switch status := known.(type) {
		case *wmistatus.Error:
			return status.Name
		case *hresult.Error:
			return status.Name
		case *rpcerrors.RPCError:
			return status.Name
		case *win32.Error:
			return status.Name
		}
	}
	return ""
}

// rpcFailure classifies an RPC failure of a session authenticated with the
// named protocol. RPC/SSP error chains can contain principal names and
// server-supplied text, so only the classification is kept.
func rpcFailure(ctx context.Context, auth, operation string, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%s: %w", operation, ctx.Err())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: %w", operation, context.DeadlineExceeded)
	}
	var (
		presentation *dcerpc.Result
		result       *hresult.Error
		wmiCode      *wmistatus.Error
		rpcCode      *rpcerrors.RPCError
		winCode      *win32.Error
		rawCode      *rpcerrors.Error
		authErr      *gssapi.Error
	)
	switch {
	case errors.As(err, &presentation):
		// Result.Error formats only the RPC result and provider reason enums.
		return diagnostic(operation + ": " + presentation.Error())
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
	case networkFailure(err) != "":
		return diagnostic(operation + ": " + networkFailure(err))
	case errors.Is(err, ntlm.ErrConfidentialityNotAvailable), errors.Is(err, ntlm.Err128BitEncryptionNotSupported):
		return diagnostic(operation + ": server did not negotiate NTLM packet privacy")
	case errors.Is(err, ntlm.ErrLogonFailure):
		return diagnostic(operation + ": NTLM authentication failed")
	case errors.As(err, &authErr):
		return diagnostic(operation + ": " + auth + " authentication failed")
	}
	return diagnostic(operation + ": RPC failed")
}

// networkFailure names a connection failure without the addresses or names
// that the underlying error text carries. It returns an empty string for other errors.
func networkFailure(err error) string {
	var (
		dnsErr *net.DNSError
		netErr net.Error
	)
	switch {
	case errors.As(err, &netErr) && netErr.Timeout():
		return "network request timed out"
	case errors.As(err, &dnsErr):
		return "could not resolve the WMI host"
	case errors.Is(err, syscall.ECONNREFUSED), errors.Is(err, syscall.Errno(10061)): // WSAECONNREFUSED
		return "connection refused"
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH),
		errors.Is(err, syscall.Errno(10065)), errors.Is(err, syscall.Errno(10051)): // WSAEHOSTUNREACH, WSAENETUNREACH
		return "host or network unreachable"
	default:
		return ""
	}
}
