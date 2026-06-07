package raw

import "github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"

// AppsLookupHandler serves a single APPS_LOOKUP service kind.
type AppsLookupHandler func(*protocol.AppsLookupRequestView, *protocol.AppsLookupBuilder) bool

func appsLookupRequestSize(pids []uint32) (int, error) {
	dirSize, err := checkedLookupMul(len(pids), protocol.LookupDirEntrySize)
	if err != nil {
		return 0, err
	}
	keySize, err := checkedLookupMul(len(pids), protocol.AppsLookupKeySize)
	if err != nil {
		return 0, err
	}
	size, err := checkedLookupAdd(protocol.AppsLookupReqHdr, dirSize)
	if err != nil {
		return 0, err
	}
	return checkedLookupAdd(size, keySize)
}

// CallAppsLookup performs a blocking typed APPS_LOOKUP call.
// The returned view is valid until the next typed call on this client.
func (c *Client) CallAppsLookup(pids []uint32) (*protocol.AppsLookupResponseView, error) {
	if err := c.validateMethod(protocol.MethodAppsLookup); err != nil {
		return nil, err
	}

	var result *protocol.AppsLookupResponseView
	err := c.callWithRetry(func() error {
		reqSize, err := appsLookupRequestSize(pids)
		if err != nil {
			return err
		}
		reqBuf := ensureClientScratch(&c.requestBuf, reqSize)
		reqLen, err := protocol.EncodeAppsLookupRequest(pids, reqBuf)
		if err != nil {
			return err
		}

		_, payload, rerr := c.doRawCall(protocol.MethodAppsLookup, reqBuf[:reqLen])
		if rerr != nil {
			return rerr
		}
		view, derr := protocol.DecodeAppsLookupResponse(payload)
		if derr != nil {
			return derr
		}
		expectedCount, err := checkedLookupU32(len(pids))
		if err != nil {
			return err
		}
		if view.ItemCount != expectedCount {
			return protocol.ErrBadItemCount
		}
		for i, expected := range pids {
			itemIndex, ierr := checkedLookupU32(i)
			if ierr != nil {
				return ierr
			}
			item, ierr := view.Item(itemIndex)
			if ierr != nil {
				return ierr
			}
			if item.Pid != expected {
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

// AppsLookupDispatch adapts a typed apps lookup handler to the raw dispatch shape.
func AppsLookupDispatch(handle AppsLookupHandler) DispatchHandler {
	if handle == nil {
		return nil
	}
	return func(request []byte, responseBuf []byte) (int, error) {
		req, err := protocol.DecodeAppsLookupRequest(request)
		if err != nil {
			return 0, err
		}
		minRequired, err := lookupMinRequired(protocol.AppsLookupRespHdr, req.ItemCount)
		if err != nil {
			return 0, err
		}
		if len(responseBuf) < minRequired {
			return 0, protocol.ErrOverflow
		}
		builder := protocol.NewAppsLookupBuilder(responseBuf, req.ItemCount, 0)
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
