// SPDX-License-Identifier: GPL-3.0-or-later

package wmiquery

import (
	"context"
	"strings"
	"time"

	"github.com/oiweiwei/go-msrpc/dcerpc"
	"github.com/oiweiwei/go-msrpc/msrpc/dcom"
	"github.com/oiweiwei/go-msrpc/msrpc/dcom/oaut"
	"github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi"
	enumerator "github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi/ienumwbemclassobject/v0"
	services "github.com/oiweiwei/go-msrpc/msrpc/dcom/wmi/iwbemservices/v0"
	"github.com/oiweiwei/go-msrpc/msrpc/dcom/wmio"
)

// Limit the number of encoded CIM objects in each RPC response.
const rowsPerPage = 16

// Query reads the caller's fixed WQL projection from root/cimv2.
func (s *DCOMSession) Query(ctx context.Context, wql string) ([]map[string]any, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer cancel()

	result, err := s.services.ExecQuery(ctx, &services.ExecQueryRequest{
		This: &s.orpc,
		QueryLanguage: &oaut.String{
			Data: "WQL",
		},
		Query: &oaut.String{
			Data: wql,
		},
		Flags: int32(wmi.GenericFlagTypeReturnImmediately | wmi.GenericFlagTypeForwardOnly),
	})
	if err != nil {
		return nil, rpcFailure(ctx, s.auth, "execute WMI query", err)
	}
	ref, err := s.reference(result.Enum.InterfacePointer())
	if err != nil {
		return nil, err
	}
	defer s.release(ref)

	// A Windows exporter can reject this interface until ExecQuery has
	// returned the first enumerator object.
	enum := s.enumeration
	if enum == nil {
		enum, err = enumerator.NewEnumClassObjectClient(ctx, s.objects)
		if err != nil {
			return nil, rpcFailure(ctx, s.auth, "open WMI enumeration interface", err)
		}
	}
	deadline, _ := s.ctx.Deadline()
	if queryDeadline, ok := ctx.Deadline(); ok && queryDeadline.Before(deadline) {
		deadline = queryDeadline
	}
	rows, err := readPages(ctx, s.auth, &s.orpc, deadline, enum.IPID(ctx, ref.IPID).Next)
	if err == nil {
		// go-msrpc may report a rejected presentation only when Next is called.
		s.enumeration = enum
	}
	return rows, err
}

type nextPage func(context.Context, *enumerator.NextRequest, ...dcerpc.CallOption) (*enumerator.NextResponse, error)

func readPages(
	ctx context.Context,
	auth string,
	orpc *dcom.ORPCThis,
	deadline time.Time,
	next nextPage,
) ([]map[string]any, error) {
	var rows []map[string]any
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, context.DeadlineExceeded
		}
		// MS-WMI Next takes a server-side timeout in milliseconds. WBEM_S_TIMEDOUT
		// may still return rows and does not indicate the end of the enumeration.
		wait := max(1, min(remaining, time.Second).Milliseconds())
		page, err := next(ctx, &enumerator.NextRequest{
			This:    orpc,
			Timeout: int32(wait),
			Count:   rowsPerPage,
		})
		if err != nil {
			return nil, rpcFailure(ctx, auth, "read WMI query results", err)
		}
		if page == nil || int(page.Returned) != len(page.Objects) || page.Returned > rowsPerPage {
			return nil, diagnostic("WMI returned an invalid enumeration page")
		}
		status := wmi.Status(page.Return)
		switch status {
		case wmi.StatusNoError:
			if page.Returned != rowsPerPage {
				return nil, diagnostic("WMI returned an incomplete enumeration page")
			}
		case wmi.StatusFalse, wmi.StatusTimedout:
		default:
			return nil, statusError("read WMI query results", uint32(page.Return))
		}

		for _, object := range page.Objects {
			values, err := decodeRow(object.InterfacePointer())
			if err != nil {
				return nil, err
			}
			rows = append(rows, values)
		}
		if status == wmi.StatusFalse {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return rows, nil
		}
	}
}

func decodeRow(pointer *dcom.InterfacePointer) (_ map[string]any, err error) {
	ref, err := decodeReference(pointer)
	if err != nil {
		return nil, err
	}
	custom, ok := ref.ObjectReference.GetValue().(*dcom.ObjectReferenceCustom)
	if !ok || !custom.ClassID.GUID().Equal(wmi.ClassObjectUnmarshalClassID.GUID()) || len(custom.ObjectData) < 4 {
		return nil, diagnostic("WMI returned an invalid class object reference")
	}
	// The WMIO decoder indexes property offsets and field positions from the wire.
	// Convert a decoder panic on a malformed object into a failed query.
	defer func() {
		if recover() != nil {
			err = diagnostic("could not decode WMI class object")
		}
	}()
	obj, err := wmio.Unmarshal(custom.ObjectData)
	if err != nil || obj.Instance == nil {
		return nil, diagnostic("could not decode WMI class object")
	}
	row := make(map[string]any, len(obj.Instance.Properties))
	for _, property := range obj.Instance.Properties {
		if property == nil || property.Nullable {
			continue
		}
		switch value := property.Value.Value.(type) {
		case string, []string, bool, int8, int16, int32, int64, uint8, uint16, uint32, uint64, float32, float64:
			row[strings.ToLower(property.Name)] = value
		}
	}
	return row, nil
}
