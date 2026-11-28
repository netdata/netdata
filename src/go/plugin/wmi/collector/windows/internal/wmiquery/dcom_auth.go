// SPDX-License-Identifier: GPL-3.0-or-later

package wmiquery

import (
	"context"

	"github.com/oiweiwei/go-msrpc/dcerpc"
	"github.com/oiweiwei/go-msrpc/ssp/credential"
	"github.com/oiweiwei/go-msrpc/ssp/gssapi"
	"github.com/oiweiwei/go-msrpc/ssp/ntlm"
)

// DCOMAuth authenticates the binds of a job's DCOM sessions. A job keeps one
// for its lifetime and opens one session at a time with it.
type DCOMAuth interface {
	// Close releases what the authenticator keeps between sessions.
	Close()

	// protocol names the authentication protocol in diagnostics.
	protocol() string
	// bindSecurity returns the security options for every bind of one session
	// to host. Every option authenticates and seals (packet privacy).
	bindSecurity(ctx context.Context, host string) ([]dcerpc.Option, error)
	// rejected reports that a session reached the host but failed to open, so
	// credentials kept for host may be what the host refused.
	rejected()
}

// NTLMAuth authenticates every bind with job-scoped NTLMv2 credentials.
type NTLMAuth struct {
	username string
	password string
}

// NewNTLMAuth accepts a DOMAIN\user, user@domain or local logon name.
func NewNTLMAuth(username, password string) NTLMAuth {
	return NTLMAuth{
		username: username,
		password: password,
	}
}

func (NTLMAuth) Close() {}

func (NTLMAuth) protocol() string { return "NTLM" }

func (a NTLMAuth) bindSecurity(_ context.Context, host string) ([]dcerpc.Option, error) {
	config := ntlm.NewConfig()
	config.NTLMVersion = ntlm.NTLMv2
	mechanism := ntlm.Mechanism{}
	creds := credential.NewFromPassword(a.username, a.password)
	return []dcerpc.Option{
		dcerpc.WithMechanism(mechanism, config),
		dcerpc.WithCredentials(gssapi.NewCredential("", []gssapi.OID{mechanism.Type()}, gssapi.InitiateOnly, creds)),
		dcerpc.WithSecurtyProvider(dcerpc.AuthTypeWinNT),
		dcerpc.WithTargetName("host/" + host),
		dcerpc.WithSeal(),
	}, nil
}

// NTLM sends the credentials with every bind; it keeps nothing to forget.
func (NTLMAuth) rejected() {}
