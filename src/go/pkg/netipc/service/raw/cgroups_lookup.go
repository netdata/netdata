package raw

import (
	"bytes"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// CgroupsLookupHandler serves a single CGROUPS_LOOKUP service kind.
type CgroupsLookupHandler func(*protocol.CgroupsLookupRequestView, *protocol.CgroupsLookupBuilder) bool

func cgroupsLookupRequestSize(paths [][]byte) (int, error) {
	dirSize, err := checkedLookupMul(len(paths), protocol.LookupDirEntrySize)
	if err != nil {
		return 0, err
	}
	size, err := checkedLookupAdd(protocol.CgroupsLookupReqHdr, dirSize)
	if err != nil {
		return 0, err
	}
	data := size
	for _, path := range paths {
		data, err = checkedLookupAlign8(data)
		if err != nil {
			return 0, err
		}
		data, err = checkedLookupAdd(data, len(path))
		if err != nil {
			return 0, err
		}
		data, err = checkedLookupAdd(data, 1)
		if err != nil {
			return 0, err
		}
	}
	return data, nil
}

// CallCgroupsLookup performs a blocking typed CGROUPS_LOOKUP call.
// The returned view is valid until the next typed call on this client.
func (c *Client) CallCgroupsLookup(paths [][]byte) (*protocol.CgroupsLookupResponseView, error) {
	if err := c.validateMethod(protocol.MethodCgroupsLookup); err != nil {
		return nil, err
	}

	var result *protocol.CgroupsLookupResponseView
	err := c.callWithRetry(func() error {
		reqSize, err := cgroupsLookupRequestSize(paths)
		if err != nil {
			return err
		}
		reqBuf := ensureClientScratch(&c.requestBuf, reqSize)
		reqLen, err := protocol.EncodeCgroupsLookupRequest(paths, reqBuf)
		if err != nil {
			return err
		}

		_, payload, rerr := c.doRawCall(protocol.MethodCgroupsLookup, reqBuf[:reqLen])
		if rerr != nil {
			return rerr
		}
		view, derr := protocol.DecodeCgroupsLookupResponse(payload)
		if derr != nil {
			return derr
		}
		expectedCount, err := checkedLookupU32(len(paths))
		if err != nil {
			return err
		}
		if view.ItemCount != expectedCount {
			return protocol.ErrBadItemCount
		}
		for i, expected := range paths {
			itemIndex, ierr := checkedLookupU32(i)
			if ierr != nil {
				return ierr
			}
			item, ierr := view.Item(itemIndex)
			if ierr != nil {
				return ierr
			}
			if !bytes.Equal(item.Path.Bytes(), expected) {
				return protocol.ErrBadLayout
			}
		}
		result = view
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CgroupsLookupDispatch adapts a typed cgroups lookup handler to the raw dispatch shape.
func CgroupsLookupDispatch(handle CgroupsLookupHandler) DispatchHandler {
	if handle == nil {
		return nil
	}
	return func(request []byte, responseBuf []byte) (int, error) {
		req, err := protocol.DecodeCgroupsLookupRequest(request)
		if err != nil {
			return 0, err
		}
		minRequired, err := lookupMinRequired(protocol.CgroupsLookupRespHdr, req.ItemCount)
		if err != nil {
			return 0, err
		}
		if len(responseBuf) < minRequired {
			return 0, protocol.ErrOverflow
		}
		builder := protocol.NewCgroupsLookupBuilder(responseBuf, req.ItemCount, 0)
		if !handle(req, builder) {
			return 0, errHandlerFailed
		}
		if builder.Error() != nil {
			return 0, builder.Error()
		}
		if builder.ItemCount() != req.ItemCount {
			return 0, protocol.ErrBadItemCount
		}
		return builder.Finish(), nil
	}
}
