// SPDX-License-Identifier: GPL-3.0-or-later

package wmiquery

import (
	"context"
	"testing"
	"time"

	"github.com/oiweiwei/go-msrpc/dcerpc"
	"github.com/oiweiwei/go-msrpc/msrpc/dcom"
	activation "github.com/oiweiwei/go-msrpc/msrpc/dcom/iactivation/v0"
	remoteunknown "github.com/oiweiwei/go-msrpc/msrpc/dcom/iremunknown/v0"
	"github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi"
	enumerator "github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi/ienumwbemclassobject/v0"
	level1login "github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi/iwbemlevel1login/v0"
	services "github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi/iwbemservices/v0"
	"github.com/oiweiwei/go-msrpc/ndr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryNegotiatesEnumerationAfterReceivingObject(t *testing.T) {
	ctx, c, peer := loggedInEnumClient(t)
	projections := []string{
		"SELECT Name FROM Win32_OperatingSystem",
		"SELECT Name FROM Win32_LogicalDisk",
	}
	for _, projection := range projections {
		rows, err := c.Query(ctx, projection)
		assert.NoError(t, err)
		assert.Empty(t, rows)
	}

	assert.Equal(t, len(projections), peer.queries)
	assert.Equal(t, []uint32{4, 5}, peer.nextIPIDs)
	assert.Equal(t, 1, peer.acceptedEnumBinds)
	assert.Zero(t, peer.rejectedEnumBinds)
}

func TestQueryDoesNotCacheRejectedEnumeration(t *testing.T) {
	ctx, c, peer := loggedInEnumClient(t)
	peer.rejectNext = true
	const query = "SELECT Name FROM Win32_OperatingSystem"
	_, err := c.Query(ctx, query)
	require.ErrorContains(t, err, "abstract syntax not supported")
	_, err = c.Query(ctx, query)
	require.NoError(t, err)
	assert.Equal(t, 2, peer.queries)
	assert.Equal(t, []uint32{5}, peer.nextIPIDs)
	assert.Equal(t, 1, peer.acceptedEnumBinds)
	assert.Equal(t, 1, peer.rejectedEnumBinds)
}

func loggedInEnumClient(t *testing.T) (context.Context, *Client, *coldEnumPeer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	peer := &coldEnumPeer{t: t}
	c := &Client{ctx: ctx, cancel: cancel, objects: peer, oxid: coldEnumOXID}
	t.Cleanup(c.Close)

	require.NoError(t, c.logIn(&activation.RemoteActivationResponse{
		RemoteUnknown: &dcom.IPID{Data1: 1},
		InterfaceData: []*dcom.InterfacePointer{
			peer.reference(level1login.Level1LoginIID, 2),
		},
	}))

	return ctx, c, peer
}

const coldEnumOXID = 17

// The peer accepts the enumeration syntax only after returning an enumerator
// object. A rejected presentation still produces a bound Conn in go-msrpc.
type coldEnumPeer struct {
	dcerpc.Conn
	t                 *testing.T
	queries           int
	rejectNext        bool
	nextIPIDs         []uint32
	acceptedEnumBinds int
	rejectedEnumBinds int
}

func (p *coldEnumPeer) Bind(ctx context.Context, opts ...dcerpc.Option) (dcerpc.Conn, error) {
	parsed, err := dcerpc.ParseOptions(ctx, append(opts, dcerpc.WithInsecure())...)
	require.NoError(p.t, err)
	require.Len(p.t, parsed.AbstractSyntaxes, 1)
	syntax := parsed.AbstractSyntaxes[0]
	bound := &coldEnumPresentation{coldEnumPeer: p, syntax: syntax}
	if syntax.Is(enumerator.EnumClassObjectSyntaxV0_0) {
		if p.queries == 0 || p.rejectNext {
			p.rejectNext = false
			p.rejectedEnumBinds++
			bound.rejection = &dcerpc.Result{
				DefResult:      dcerpc.ProviderRejection,
				ProviderReason: dcerpc.AbstractSyntaxNotSupported,
			}
		} else {
			p.acceptedEnumBinds++
		}
	}
	return bound, nil
}

func (p *coldEnumPeer) Close(context.Context) error { return nil }

func (p *coldEnumPeer) reference(iid *dcom.IID, ipid uint32) *dcom.InterfacePointer {
	p.t.Helper()
	ref := &dcom.ObjectReference{
		Signature: []byte(dcom.ObjectReferenceCustomSignature),
		Flags:     dcom.ObjectReferenceTypeStandard,
		IID:       iid,
		ObjectReference: &dcom.ObjectReference_ObjectReference{
			Value: &dcom.ObjectReference_Standard{Standard: &dcom.ObjectReferenceStandard{
				Std: &dcom.StdObjectReference{
					OXID:                  coldEnumOXID,
					OID:                   uint64(ipid),
					IPID:                  &dcom.IPID{Data1: ipid},
					PublicReferencesCount: 1,
				},
				ResolverAddr: &dcom.DualStringArray{SecurityOffset: 1, StringArray: []uint16{0, 0}},
			}},
		},
	}
	wire, err := ndr.Marshal(ref, ndr.Opaque)
	require.NoError(p.t, err)
	require.NoError(p.t, ndr.Unmarshal(wire, &dcom.ObjectReference{}, ndr.Opaque))
	return &dcom.InterfacePointer{Data: wire}
}

type coldEnumPresentation struct {
	*coldEnumPeer
	syntax    *dcerpc.SyntaxID
	rejection error
}

func (p *coldEnumPresentation) Invoke(ctx context.Context, op dcerpc.Operation, opts ...dcerpc.CallOption) error {
	if p.rejection != nil {
		return p.rejection
	}
	ipid, ok := dcom.HasIPID(opts)
	require.True(p.t, ok, op.OpName())

	var response ndr.Marshaler
	switch op.OpName() {
	case (&level1login.NTLMLoginRequest{}).OpName():
		require.True(p.t, p.syntax.Is(level1login.Level1LoginSyntaxV0_0))
		response = &level1login.NTLMLoginResponse{
			Namespace: (*wmi.Services)(p.reference(services.ServicesIID, 3)),
		}
	case (&services.ExecQueryRequest{}).OpName():
		require.True(p.t, p.syntax.Is(services.ServicesSyntaxV0_0))
		response = &services.ExecQueryResponse{
			Enum: (*wmi.EnumClassObject)(p.reference(enumerator.EnumClassObjectIID, uint32(4+p.queries))),
		}
		p.queries++
	case (&enumerator.NextRequest{}).OpName():
		require.True(p.t, p.syntax.Is(enumerator.EnumClassObjectSyntaxV0_0))
		p.nextIPIDs = append(p.nextIPIDs, ipid.Data1)
		response = &enumerator.NextResponse{
			Count:  rowsPerPage,
			Return: int32(wmi.StatusFalse),
		}
	case (&remoteunknown.RemoteReleaseRequest{}).OpName():
		require.True(p.t, p.syntax.Is(remoteunknown.RemoteUnknownSyntaxV0_0))
		response = &remoteunknown.RemoteReleaseResponse{}
	default:
		p.t.Fatalf("unexpected RPC operation %s", op.OpName())
	}
	wire, err := ndr.Marshal(response)
	require.NoError(p.t, err)
	return op.UnmarshalNDRResponse(ctx, ndr.NDR20(wire))
}
