// SPDX-License-Identifier: GPL-3.0-or-later

package wmiquery

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/google/uuid"
	"github.com/oiweiwei/go-msrpc/dcerpc"
	rpcuuid "github.com/oiweiwei/go-msrpc/midl/uuid"
	"github.com/oiweiwei/go-msrpc/msrpc/dcetypes"
	"github.com/oiweiwei/go-msrpc/msrpc/dcom"
	activation "github.com/oiweiwei/go-msrpc/msrpc/dcom/iactivation/v0"
	exporter "github.com/oiweiwei/go-msrpc/msrpc/dcom/iobjectexporter/v0"
	remoteunknown "github.com/oiweiwei/go-msrpc/msrpc/dcom/iremunknown/v0"
	"github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi"
	enumerator "github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi/ienumwbemclassobject/v0"
	level1login "github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi/iwbemlevel1login/v0"
	services "github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi/iwbemservices/v0"
	"github.com/oiweiwei/go-msrpc/msrpc/dtyp"
	"github.com/oiweiwei/go-msrpc/ndr"
)

// Client holds a single collection cycle's WMI session. Calls are serial.
type Client struct {
	ctx         context.Context
	cancel      context.CancelFunc
	dialer      rpcDialer
	resolver    dcerpc.Conn
	objects     dcerpc.Conn
	orpc        dcom.ORPCThis
	unknown     remoteunknown.RemoteUnknownClient
	services    services.ServicesClient
	enumeration enumerator.EnumClassObjectClient
	oxid        uint64
	refs        []*dcom.RemoteInterfaceReference
}

// Open activates WMI on the configured host and opens its root/cimv2 namespace.
// The caller's deadline bounds activation, queries, and release.
func Open(ctx context.Context, host, username, password, auth string) (*Client, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, diagnostic("WMI session requires a collection deadline")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !ValidHost(host) {
		return nil, diagnostic("WMI host must be a hostname or IP address")
	}

	ctx, cancel := context.WithCancel(ctx)
	c := &Client{
		ctx:    ctx,
		cancel: cancel,
		dialer: rpcDialer{
			host: host,
			ctx:  ctx,
			dial: (&net.Dialer{}).DialContext,
		},
	}
	if err := c.open(host, username, password, auth); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) open(host, username, password, auth string) error {
	ctx := c.ctx
	// An IPv6 zone selects an interface on the poller. Only the socket dialer
	// needs it; the DCOM address parser and NTLM target use the remote address.
	host, _, _ = strings.Cut(host, "%")
	security, err := securityOptions(host, username, password, auth)
	if err != nil {
		return err
	}
	cid, _ := rpcuuid.Parse(uuid.NewString())
	c.orpc.CID = (*dcom.CID)(dtyp.GUIDFromUUID(cid))

	c.resolver, err = c.dial(host, security, dcerpc.WithEndpoint("ncacn_ip_tcp:[135]"))
	if err != nil {
		return rpcFailure(ctx, "open DCOM resolver", err)
	}
	// Windows can reject additional interfaces in a combined bind. Each generated
	// client negotiates its own interface on the existing endpoint transport.
	resolver, err := exporter.NewObjectExporterClient(ctx, c.resolver)
	if err != nil {
		return rpcFailure(ctx, "open DCOM object resolver interface", err)
	}
	alive, err := resolver.ServerAlive2(ctx, &exporter.ServerAlive2Request{})
	if err != nil {
		return rpcFailure(ctx, "read DCOM server version", err)
	}
	if alive.COMVersion == nil {
		return diagnostic("DCOM resolver returned no COM version")
	}
	c.orpc.Version = alive.COMVersion

	activator, err := activation.NewActivationClient(ctx, c.resolver)
	if err != nil {
		return rpcFailure(ctx, "open DCOM activation interface", err)
	}
	act, err := activator.RemoteActivation(ctx, &activation.RemoteActivationRequest{
		ORPCThis:                   &c.orpc,
		ClassID:                    wmi.Level1LoginClassID.GUID(),
		IIDs:                       []*dcom.IID{level1login.Level1LoginIID},
		RequestedProtocolSequences: []uint16{uint16(dcetypes.ProtocolTCP)},
	})
	if err != nil {
		return rpcFailure(ctx, "activate WMI", err)
	}
	if act.HResult != 0 {
		return statusError("activate WMI", uint32(act.HResult))
	}
	if len(act.Results) != 1 || len(act.InterfaceData) != 1 {
		return diagnostic("DCOM activation returned an invalid WMI interface count")
	}
	if act.Results[0] != 0 {
		return statusError("activate WMI interface", uint32(act.Results[0]))
	}
	if act.RemoteUnknown.GUID().IsZero() {
		return diagnostic("DCOM activation returned no remote release interface")
	}
	c.oxid = act.OXID

	endpoints := tcpEndpoints(act.OXIDBindings)
	if len(endpoints) == 0 {
		resolved, err := resolver.ResolveOxid2(ctx, &exporter.ResolveOxid2Request{
			OXID:                       act.OXID,
			RequestedProtocolSequences: []uint16{uint16(dcetypes.ProtocolTCP)},
		})
		if err != nil {
			return rpcFailure(ctx, "resolve WMI exporter endpoint", err)
		}
		endpoints = tcpEndpoints(resolved.OXIDBindings)
	}
	if len(endpoints) == 0 {
		return diagnostic("DCOM activation returned no WMI TCP endpoint")
	}

	c.objects, err = c.dial(host, security, endpoints...)
	if err != nil {
		return rpcFailure(ctx, "open WMI object transport", err)
	}
	return c.logIn(act)
}

func (c *Client) logIn(act *activation.RemoteActivationResponse) error {
	ctx := c.ctx
	var err error
	c.unknown, err = remoteunknown.NewRemoteUnknownClient(ctx, c.objects, dcom.WithIPID(act.RemoteUnknown))
	if err != nil {
		return rpcFailure(ctx, "open WMI release interface", err)
	}

	loginRef, err := c.reference(act.InterfaceData[0])
	if err != nil {
		return err
	}
	c.refs = append(c.refs, loginRef)

	login, err := level1login.NewLevel1LoginClient(ctx, c.objects, dcom.WithIPID(loginRef.IPID))
	if err != nil {
		return rpcFailure(ctx, "open WMI login interface", err)
	}
	// NTLMLogin opens the namespace using the authenticated RPC caller identity.
	namespace, err := login.NTLMLogin(ctx, &level1login.NTLMLoginRequest{
		This:            &c.orpc,
		NetworkResource: "//./root/cimv2",
	})
	if err != nil {
		return rpcFailure(ctx, "open WMI root/cimv2 namespace", err)
	}

	nsRef, err := c.reference(namespace.Namespace.InterfacePointer())
	if err != nil {
		return err
	}
	c.refs = append(c.refs, nsRef)

	c.services, err = services.NewServicesClient(ctx, c.objects, dcom.WithIPID(nsRef.IPID))
	if err != nil {
		return rpcFailure(ctx, "open WMI services interface", err)
	}
	c.enumeration, err = enumerator.NewEnumClassObjectClient(ctx, c.objects)
	if err != nil {
		return rpcFailure(ctx, "open WMI enumeration interface", err)
	}
	return nil
}

func (c *Client) dial(host string, security []dcerpc.Option, endpoints ...dcerpc.Option) (dcerpc.Conn, error) {
	options := []dcerpc.Option{
		dcerpc.WithDialer(&c.dialer),
		dcerpc.WithDNSResolver(nil),
	}
	options = append(options, security...)
	options = append(options, endpoints...)
	return dcerpc.Dial(c.ctx, host, options...)
}

// tcpEndpoints uses only the TCP ports advertised by the configured target.
// An exporter can advertise hostnames or addresses for its other interfaces.
func tcpEndpoints(bindings *dcom.DualStringArray) []dcerpc.Option {
	if bindings == nil || int(bindings.SecurityOffset) > len(bindings.StringArray) {
		return nil
	}

	var endpoints []dcerpc.Option
	for fields := bindings.StringArray[:bindings.SecurityOffset]; len(fields) > 1; {
		tower := fields[0]
		fields = fields[1:]
		end := 0
		for end < len(fields) && fields[end] != 0 {
			end++
		}
		if end == len(fields) {
			break
		}
		if int(tower) == dcetypes.ProtocolTCP {
			binding, err := dcerpc.ParseStringBinding("ncacn_ip_tcp:" + string(utf16.Decode(fields[:end])))
			if err == nil {
				if port, err := strconv.ParseUint(binding.Endpoint, 10, 16); err == nil && port != 0 {
					endpoints = append(endpoints, dcerpc.WithEndpoint("ncacn_ip_tcp:["+strconv.FormatUint(port, 10)+"]"))
				}
			}
		}
		fields = fields[end+1:]
	}
	return endpoints
}

func (c *Client) reference(pointer *dcom.InterfacePointer) (*dcom.RemoteInterfaceReference, error) {
	obj, err := decodeReference(pointer)
	if err != nil {
		return nil, err
	}
	ref, ok := obj.ObjectReference.GetValue().(*dcom.ObjectReferenceStandard)
	if !ok || ref.Std == nil || ref.Std.IPID.GUID().IsZero() {
		return nil, diagnostic("WMI returned an invalid standard object reference")
	}
	if ref.Std.OXID != c.oxid {
		return nil, diagnostic("WMI returned an object reference for a different exporter")
	}
	return &dcom.RemoteInterfaceReference{
		IPID:                  ref.Std.IPID,
		PublicReferencesCount: ref.Std.PublicReferencesCount,
	}, nil
}

func decodeReference(pointer *dcom.InterfacePointer) (*dcom.ObjectReference, error) {
	if pointer == nil || len(pointer.Data) < 24 || string(pointer.Data[:4]) != dcom.ObjectReferenceCustomSignature {
		return nil, diagnostic("WMI returned an invalid DCOM object reference")
	}
	obj := &dcom.ObjectReference{}
	if err := ndr.Unmarshal(pointer.Data, obj, ndr.Opaque); err != nil {
		return nil, diagnostic("could not decode WMI DCOM object reference")
	}
	return obj, nil
}

func (c *Client) release(refs ...*dcom.RemoteInterfaceReference) {
	if len(refs) == 0 || c.unknown == nil || c.ctx.Err() != nil {
		return
	}
	// Keep a stalled release from consuming the remaining query budget.
	ctx, cancel := context.WithTimeout(c.ctx, time.Second)
	defer cancel()
	_, _ = c.unknown.RemoteRelease(ctx, &remoteunknown.RemoteReleaseRequest{
		This:                     &c.orpc,
		InterfaceReferencesCount: uint16(len(refs)),
		InterfaceReferences:      refs,
	})
}

// Close releases the outstanding references when the exporter remains reachable
// within the cycle deadline and closes every socket, including failed binds.
func (c *Client) Close() {
	c.release(c.refs...)
	c.refs = nil
	c.unknown = nil
	c.services = nil
	c.enumeration = nil
	c.cancel()
	c.dialer.close()
	if c.objects != nil {
		_ = c.objects.Close(c.ctx)
		c.objects = nil
	}
	if c.resolver != nil {
		_ = c.resolver.Close(c.ctx)
		c.resolver = nil
	}
}
