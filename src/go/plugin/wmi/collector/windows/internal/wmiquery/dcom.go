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

const dcomEndpointMapper = "ncacn_ip_tcp:[135]"

// DCOMSession is one collection cycle's WMI session over DCOM. Calls are serial.
type DCOMSession struct {
	ctx         context.Context
	cancel      context.CancelFunc
	auth        string
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

// OpenDCOM activates WMI on host and opens its root/cimv2 namespace with the
// job's authenticator and packet privacy. The caller validates host and
// credentials. The context deadline bounds authentication, activation, every
// query and the release in Close.
func OpenDCOM(ctx context.Context, host string, auth DCOMAuth) (*DCOMSession, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, diagnostic("WMI session requires a collection deadline")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	s := &DCOMSession{
		ctx:    ctx,
		cancel: cancel,
		auth:   auth.protocol(),
		dialer: rpcDialer{
			host: host,
			ctx:  ctx,
			dial: (&net.Dialer{}).DialContext,
		},
	}
	if err := s.open(host, auth); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// open connects and tells auth when the host was reached but the session
// failed, which may be the host refusing its credentials. A session that
// failed before any socket opened, or ran out of time, never got an answer.
func (s *DCOMSession) open(host string, auth DCOMAuth) error {
	err := s.connect(host, auth)
	if err != nil && len(s.dialer.sockets) > 0 && s.ctx.Err() == nil {
		auth.rejected()
	}
	return err
}

func (s *DCOMSession) connect(host string, auth DCOMAuth) error {
	ctx := s.ctx
	// An IPv6 zone selects an interface on the poller. Only the socket dialer
	// needs it; the DCOM address parser and the authentication target use the
	// remote address.
	host, _, _ = strings.Cut(host, "%")
	security, err := auth.bindSecurity(ctx, host)
	if err != nil {
		return err
	}
	cid, _ := rpcuuid.Parse(uuid.NewString())
	s.orpc.CID = (*dcom.CID)(dtyp.GUIDFromUUID(cid))

	s.resolver, err = s.dial(host, security, dcerpc.WithEndpoint(dcomEndpointMapper))
	if err != nil {
		return rpcFailure(ctx, s.auth, "open DCOM resolver", err)
	}
	// Windows can reject additional interfaces in a combined bind. Each generated
	// client negotiates its own interface on the existing endpoint transport.
	resolver, err := exporter.NewObjectExporterClient(ctx, s.resolver)
	if err != nil {
		return rpcFailure(ctx, s.auth, "open DCOM object resolver interface", err)
	}
	alive, err := resolver.ServerAlive2(ctx, &exporter.ServerAlive2Request{})
	if err != nil {
		return rpcFailure(ctx, s.auth, "read DCOM server version", err)
	}
	if alive.COMVersion == nil {
		return diagnostic("DCOM resolver returned no COM version")
	}
	s.orpc.Version = alive.COMVersion

	activator, err := activation.NewActivationClient(ctx, s.resolver)
	if err != nil {
		return rpcFailure(ctx, s.auth, "open DCOM activation interface", err)
	}
	act, err := activator.RemoteActivation(ctx, &activation.RemoteActivationRequest{
		ORPCThis:                   &s.orpc,
		ClassID:                    wmi.Level1LoginClassID.GUID(),
		IIDs:                       []*dcom.IID{level1login.Level1LoginIID},
		RequestedProtocolSequences: []uint16{uint16(dcetypes.ProtocolTCP)},
	})
	if err != nil {
		return rpcFailure(ctx, s.auth, "activate WMI", err)
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
	s.oxid = act.OXID

	endpoints := tcpEndpoints(act.OXIDBindings)
	if len(endpoints) == 0 {
		resolved, err := resolver.ResolveOxid2(ctx, &exporter.ResolveOxid2Request{
			OXID:                       act.OXID,
			RequestedProtocolSequences: []uint16{uint16(dcetypes.ProtocolTCP)},
		})
		if err != nil {
			return rpcFailure(ctx, s.auth, "resolve WMI exporter endpoint", err)
		}
		endpoints = tcpEndpoints(resolved.OXIDBindings)
	}
	if len(endpoints) == 0 {
		return diagnostic("DCOM activation returned no WMI TCP endpoint")
	}

	s.objects, err = s.dial(host, security, endpoints...)
	if err != nil {
		return rpcFailure(ctx, s.auth, "open WMI object transport", err)
	}
	return s.login(act)
}

func (s *DCOMSession) login(act *activation.RemoteActivationResponse) error {
	ctx := s.ctx
	var err error
	s.unknown, err = remoteunknown.NewRemoteUnknownClient(ctx, s.objects, dcom.WithIPID(act.RemoteUnknown))
	if err != nil {
		return rpcFailure(ctx, s.auth, "open WMI release interface", err)
	}

	loginRef, err := s.reference(act.InterfaceData[0])
	if err != nil {
		return err
	}
	s.refs = append(s.refs, loginRef)

	login, err := level1login.NewLevel1LoginClient(ctx, s.objects, dcom.WithIPID(loginRef.IPID))
	if err != nil {
		return rpcFailure(ctx, s.auth, "open WMI login interface", err)
	}
	// Despite its name, NTLMLogin opens the namespace as the authenticated RPC
	// caller, whichever protocol authenticated it.
	namespace, err := login.NTLMLogin(ctx, &level1login.NTLMLoginRequest{
		This:            &s.orpc,
		NetworkResource: "//./root/cimv2",
	})
	if err != nil {
		return rpcFailure(ctx, s.auth, "open WMI root/cimv2 namespace", err)
	}

	nsRef, err := s.reference(namespace.Namespace.InterfacePointer())
	if err != nil {
		return err
	}
	s.refs = append(s.refs, nsRef)

	s.services, err = services.NewServicesClient(ctx, s.objects, dcom.WithIPID(nsRef.IPID))
	if err != nil {
		return rpcFailure(ctx, s.auth, "open WMI services interface", err)
	}
	return nil
}

func (s *DCOMSession) dial(host string, security []dcerpc.Option, endpoints ...dcerpc.Option) (dcerpc.Conn, error) {
	options := []dcerpc.Option{
		dcerpc.WithDialer(&s.dialer),
		dcerpc.WithDNSResolver(nil),
	}
	options = append(options, security...)
	options = append(options, endpoints...)
	return dcerpc.Dial(s.ctx, host, options...)
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
					endpoints = append(
						endpoints,
						dcerpc.WithEndpoint("ncacn_ip_tcp:["+strconv.FormatUint(port, 10)+"]"),
					)
				}
			}
		}
		fields = fields[end+1:]
	}
	return endpoints
}

func (s *DCOMSession) reference(pointer *dcom.InterfacePointer) (*dcom.RemoteInterfaceReference, error) {
	obj, err := decodeReference(pointer)
	if err != nil {
		return nil, err
	}
	ref, ok := obj.ObjectReference.GetValue().(*dcom.ObjectReferenceStandard)
	if !ok || ref.Std == nil || ref.Std.IPID.GUID().IsZero() {
		return nil, diagnostic("WMI returned an invalid standard object reference")
	}
	if ref.Std.OXID != s.oxid {
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

func (s *DCOMSession) release(refs ...*dcom.RemoteInterfaceReference) {
	if len(refs) == 0 || s.unknown == nil || s.ctx.Err() != nil {
		return
	}
	// Keep a stalled release from consuming the remaining query budget.
	ctx, cancel := context.WithTimeout(s.ctx, time.Second)
	defer cancel()
	_, _ = s.unknown.RemoteRelease(ctx, &remoteunknown.RemoteReleaseRequest{
		This:                     &s.orpc,
		InterfaceReferencesCount: uint16(len(refs)),
		InterfaceReferences:      refs,
	})
}

// Close releases the outstanding references when the exporter remains reachable
// within the cycle deadline and closes every socket, including failed binds.
func (s *DCOMSession) Close() {
	s.release(s.refs...)
	s.refs = nil
	s.unknown = nil
	s.services = nil
	s.enumeration = nil
	s.cancel()
	s.dialer.close()
	if s.objects != nil {
		_ = s.objects.Close(s.ctx)
		s.objects = nil
	}
	if s.resolver != nil {
		_ = s.resolver.Close(s.ctx)
		s.resolver = nil
	}
}
