// SPDX-License-Identifier: GPL-3.0-or-later

package wmiquery

import (
	"context"
	"testing"
	"time"

	"github.com/oiweiwei/go-msrpc/dcerpc"
	"github.com/oiweiwei/go-msrpc/msrpc/dcom"
	"github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi"
	enumerator "github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi/ienumwbemclassobject/v0"
	"github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi/iwbemclassobject/v0"
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
		return &enumerator.NextResponse{Return: int32(wmi.StatusFalse)}, nil
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
		return &enumerator.NextResponse{Return: int32(wmi.StatusFalse), Returned: 1}, nil
	})
	assert.Nil(t, rows)
	assert.Contains(t, SafeMessage(err), "invalid enumeration page")
}

func testClassObject(t *testing.T, name string, counter uint64) *wmi.ClassObject {
	t.Helper()
	class := &wmio.Object{Class: &wmio.ObjectClass{CurrentClass: wmio.Class{
		Name: "Win32_PerfFormattedData_Tcpip_NetworkInterface",
		Properties: []*wmio.Property{
			{Name: "Name", Order: 0, Value: wmio.Value{Type: wmio.String, Value: ""}},
			{Name: "BytesReceivedPersec", Order: 1, Value: wmio.Value{Type: wmio.Uint64, Value: uint64(0)}},
			{Name: "BytesSentPersec", Order: 2, Value: wmio.Value{Type: wmio.Uint64, Value: uint64(123)}},
		},
	}}}
	instance, err := class.New(wmio.Values{"Name": name, "BytesReceivedPersec": counter, "BytesSentPersec": nil})
	require.NoError(t, err)
	data, err := wmio.Marshal(instance)
	require.NoError(t, err)
	ref := &dcom.ObjectReference{
		Signature: []byte(dcom.ObjectReferenceCustomSignature),
		Flags:     dcom.ObjectReferenceTypeCustom,
		IID:       iwbemclassobject.ClassObjectIID,
		ObjectReference: &dcom.ObjectReference_ObjectReference{
			Value: &dcom.ObjectReference_Custom{Custom: &dcom.ObjectReferenceCustom{
				ClassID:    wmi.ClassObjectUnmarshalClassID,
				ObjectData: data,
			}},
		},
	}
	buf, err := ndr.Marshal(ref, ndr.Opaque)
	require.NoError(t, err)
	return &wmi.ClassObject{Data: buf}
}
