// SPDX-License-Identifier: GPL-3.0-or-later

package wmiquery

import (
	"context"
	"net"

	"github.com/oiweiwei/go-msrpc/dcerpc"
	"github.com/oiweiwei/go-msrpc/ssp/credential"
	"github.com/oiweiwei/go-msrpc/ssp/gssapi"
	"github.com/oiweiwei/go-msrpc/ssp/ntlm"
)

// securityOptions authenticates every bind with job-scoped NTLMv2 credentials
// and seals the traffic (packet privacy).
func securityOptions(host, username, password string) []dcerpc.Option {
	config := ntlm.NewConfig()
	config.NTLMVersion = ntlm.NTLMv2
	mechanism := ntlm.Mechanism{}
	creds := credential.NewFromPassword(username, password)
	return []dcerpc.Option{
		dcerpc.WithMechanism(mechanism, config),
		dcerpc.WithCredentials(gssapi.NewCredential("", []gssapi.OID{mechanism.Type()}, gssapi.InitiateOnly, creds)),
		dcerpc.WithSecurtyProvider(dcerpc.AuthTypeWinNT),
		dcerpc.WithTargetName("host/" + host),
		dcerpc.WithSeal(),
	}
}

// rpcDialer owns every TCP socket in a collection cycle. The advertised WMI ports
// must be reached on the same address as the activation call.
type rpcDialer struct {
	host    string
	ctx     context.Context
	dial    func(context.Context, string, string) (net.Conn, error)
	pinned  string
	sockets []rpcSocket
}

type rpcSocket struct {
	conn net.Conn
	stop func() bool
}

func (d *rpcDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	host := d.pinned
	if host == "" {
		host = d.host
	}
	conn, err := d.dial(ctx, network, net.JoinHostPort(host, port))
	if err != nil {
		return nil, err
	}
	if deadline, ok := d.ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	d.pinned, _, _ = net.SplitHostPort(conn.RemoteAddr().String())
	stop := context.AfterFunc(d.ctx, func() { _ = conn.Close() })
	d.sockets = append(d.sockets, rpcSocket{
		conn: conn,
		stop: stop,
	})
	return conn, nil
}

func (d *rpcDialer) close() {
	for _, socket := range d.sockets {
		socket.stop()
		_ = socket.conn.Close()
	}
	d.sockets = nil
}
