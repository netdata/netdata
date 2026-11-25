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
	"github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi/iwbemclassobject/v0"
	level1login "github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi/iwbemlevel1login/v0"
	services "github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi/iwbemservices/v0"
	"github.com/oiweiwei/go-msrpc/msrpc/dcom/wmio"
	"github.com/oiweiwei/go-msrpc/ndr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadPagesPreservesRowsOnTimeoutAndFinalPage(t *testing.T) {
	first := testClassObject(t, "adapter 1", uint64(0))
	second := testClassObject(t, "adapter 2", 1<<63+1)
	pages := []*enumerator.NextResponse{
		{Return: int32(wmi.StatusTimedout)},
		{Return: int32(wmi.StatusTimedout), Returned: 1, Objects: []*wmi.ClassObject{first}},
		{Return: int32(wmi.StatusFalse), Returned: 1, Objects: []*wmi.ClassObject{second}},
	}
	calls := 0
	deadline := time.Now().Add(time.Second)
	rows, err := readPages(t.Context(), &dcom.ORPCThis{}, deadline, func(
		_ context.Context, request *enumerator.NextRequest, _ ...dcerpc.CallOption,
	) (*enumerator.NextResponse, error) {
		assert.Positive(t, request.Timeout)
		assert.LessOrEqual(t, request.Timeout, int32(1000))
		assert.EqualValues(t, rowsPerPage, request.Count)
		require.Less(t, calls, len(pages))
		page := pages[calls]
		calls++
		return page, nil
	})
	require.NoError(t, err)
	assert.Equal(t, len(pages), calls)
	assert.Equal(t, []map[string]any{
		{"name": "adapter 1", "bytesreceivedpersec": uint64(0)},
		{"name": "adapter 2", "bytesreceivedpersec": uint64(1<<63 + 1)},
	}, rows)
}

func TestReadPagesEmptyResult(t *testing.T) {
	rows, err := readPages(t.Context(), &dcom.ORPCThis{}, time.Now().Add(time.Second), func(
		_ context.Context, _ *enumerator.NextRequest, _ ...dcerpc.CallOption,
	) (*enumerator.NextResponse, error) {
		return &enumerator.NextResponse{
			Return: int32(wmi.StatusFalse),
		}, nil
	})
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestReadPagesDiscardsPartialRowsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	object := testClassObject(t, "adapter 1", 1)
	calls := 0
	rows, err := readPages(ctx, &dcom.ORPCThis{}, time.Now().Add(time.Second), func(
		_ context.Context, _ *enumerator.NextRequest, _ ...dcerpc.CallOption,
	) (*enumerator.NextResponse, error) {
		calls++
		if calls == 1 {
			return &enumerator.NextResponse{
				Return:   int32(wmi.StatusTimedout),
				Returned: 1,
				Objects:  []*wmi.ClassObject{object},
			}, nil
		}
		cancel()
		return nil, ctx.Err()
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, rows)
}

func TestReadPagesRejectsMalformedPage(t *testing.T) {
	rows, err := readPages(t.Context(), &dcom.ORPCThis{}, time.Now().Add(time.Second), func(
		_ context.Context, _ *enumerator.NextRequest, _ ...dcerpc.CallOption,
	) (*enumerator.NextResponse, error) {
		return &enumerator.NextResponse{
			Return:   int32(wmi.StatusFalse),
			Returned: 1,
		}, nil
	})
	assert.Nil(t, rows)
	assert.Contains(t, SafeMessage(err), "invalid enumeration page")
}

func testClassObject(t *testing.T, name string, counter uint64) *wmi.ClassObject {
	t.Helper()
	class := &wmio.Object{
		Class: &wmio.ObjectClass{
			CurrentClass: wmio.Class{
				Name: "Win32_PerfFormattedData_Tcpip_NetworkInterface",
				Properties: []*wmio.Property{
					{Name: "Name", Order: 0, Value: wmio.Value{
						Type:  wmio.String,
						Value: "",
					}},
					{Name: "BytesReceivedPersec", Order: 1, Value: wmio.Value{
						Type:  wmio.Uint64,
						Value: uint64(0),
					}},
					{Name: "BytesSentPersec", Order: 2, Value: wmio.Value{
						Type:  wmio.Uint64,
						Value: uint64(123),
					}},
				},
			},
		},
	}
	instance, err := class.New(wmio.Values{
		"Name":                name,
		"BytesReceivedPersec": counter,
		"BytesSentPersec":     nil,
	})
	require.NoError(t, err)
	data, err := wmio.Marshal(instance)
	require.NoError(t, err)
	ref := &dcom.ObjectReference{
		Signature: []byte(dcom.ObjectReferenceCustomSignature),
		Flags:     dcom.ObjectReferenceTypeCustom,
		IID:       iwbemclassobject.ClassObjectIID,
		ObjectReference: &dcom.ObjectReference_ObjectReference{
			Value: &dcom.ObjectReference_Custom{
				Custom: &dcom.ObjectReferenceCustom{
					ClassID:    wmi.ClassObjectUnmarshalClassID,
					ObjectData: data,
				},
			},
		},
	}
	buf, err := ndr.Marshal(ref, ndr.Opaque)
	require.NoError(t, err)
	return &wmi.ClassObject{
		Data: buf,
	}
}

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

func loggedInEnumClient(t *testing.T) (context.Context, *DCOMSession, *coldEnumPeer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	peer := &coldEnumPeer{
		t: t,
	}
	c := &DCOMSession{
		ctx:     ctx,
		cancel:  cancel,
		objects: peer,
		oxid:    coldEnumOXID,
	}
	t.Cleanup(c.Close)

	require.NoError(t, c.login(&activation.RemoteActivationResponse{
		RemoteUnknown: &dcom.IPID{
			Data1: 1,
		},
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
	bound := &coldEnumPresentation{
		coldEnumPeer: p,
		syntax:       syntax,
	}
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
			Value: &dcom.ObjectReference_Standard{
				Standard: &dcom.ObjectReferenceStandard{
					Std: &dcom.StdObjectReference{
						OXID: coldEnumOXID,
						OID:  uint64(ipid),
						IPID: &dcom.IPID{
							Data1: ipid,
						},
						PublicReferencesCount: 1,
					},
					ResolverAddr: &dcom.DualStringArray{
						SecurityOffset: 1,
						StringArray:    []uint16{0, 0},
					},
				},
			},
		},
	}
	wire, err := ndr.Marshal(ref, ndr.Opaque)
	require.NoError(p.t, err)
	require.NoError(p.t, ndr.Unmarshal(wire, &dcom.ObjectReference{}, ndr.Opaque))
	return &dcom.InterfacePointer{
		Data: wire,
	}
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
