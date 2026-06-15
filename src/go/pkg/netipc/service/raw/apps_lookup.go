package raw

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// AppsLookupHandler serves a single APPS_LOOKUP service kind.
type AppsLookupHandler func(*protocol.AppsLookupRequestView, *protocol.AppsLookupBuilder) bool

func appsLookupRequestSize(pids []uint32) (int, error) {
	return appsLookupRequestSizeForCount(len(pids))
}

func appsLookupRequestSizeForCount(count int) (int, error) {
	dirSize, err := checkedLookupMul(count, protocol.LookupDirEntrySize)
	if err != nil {
		return 0, err
	}
	keySize, err := checkedLookupMul(count, protocol.AppsLookupKeySize)
	if err != nil {
		return 0, err
	}
	size, err := checkedLookupAdd(protocol.AppsLookupReqHdr, dirSize)
	if err != nil {
		return 0, err
	}
	return checkedLookupAdd(size, keySize)
}

func appsLookupNextRequest(pids []uint32, maxPayload uint32) (int, int, error) {
	if len(pids) == 0 {
		size, err := appsLookupRequestSize(nil)
		return 0, size, err
	}
	if maxPayload == 0 {
		maxPayload = protocol.MaxPayloadDefault
	}
	perItem := protocol.LookupDirEntrySize + protocol.AppsLookupKeySize
	if maxPayload < uint32(protocol.AppsLookupReqHdr+perItem) {
		return 0, 0, protocol.ErrOverflow
	}
	maxCount := int(maxPayload-uint32(protocol.AppsLookupReqHdr)) / perItem
	if maxCount <= 0 {
		return 0, 0, protocol.ErrOverflow
	}
	count := len(pids)
	if count > maxCount {
		count = maxCount
	}
	size, err := appsLookupRequestSize(pids[:count])
	if err != nil {
		return 0, 0, err
	}
	return count, size, nil
}

// CallAppsLookup performs a blocking typed APPS_LOOKUP call.
// The returned view is valid until the next typed call on this client.
func (c *Client) CallAppsLookup(pids []uint32) (*protocol.AppsLookupResponseView, error) {
	return c.CallAppsLookupWithTimeout(pids, 0)
}

func (c *Client) CallAppsLookupWithTimeout(pids []uint32, timeoutMs uint32) (*protocol.AppsLookupResponseView, error) {
	if err := c.validateMethod(protocol.MethodAppsLookup); err != nil {
		return nil, err
	}
	if uint64(len(pids)) > uint64(c.maxLogicalLookupItems) {
		return nil, protocol.ErrOverflow
	}
	if err := c.ensureReadyForLogicalLookup(); err != nil {
		return nil, err
	}

	expectedCount, err := checkedLookupU32(len(pids))
	if err != nil {
		return nil, err
	}
	rawItems := make([][]byte, 0, len(pids))
	start := 0
	var generation uint64
	haveGeneration := false
	subcalls := uint32(0)
	deadline := c.newLookupDeadline(timeoutMs)
	for {
		reqCount, reqSize, err := appsLookupNextRequest(pids[start:], c.sessionMaxRequestPayloadBytes())
		if err != nil {
			oneItemSize, serr := appsLookupRequestSize([]uint32{0})
			if errors.Is(err, protocol.ErrOverflow) && start < len(pids) && serr == nil {
				if cerr := c.ensureLookupRequestCapacity(oneItemSize); cerr == nil {
					continue
				}
			}
			return nil, err
		}
		reqPids := pids[start : start+reqCount]
		reqBuf := ensureClientScratch(&c.requestBuf, reqSize)
		reqLen, err := protocol.EncodeAppsLookupRequest(reqPids, reqBuf)
		if err != nil {
			return nil, err
		}

		remainingTimeout, terr := c.lookupRemainingTimeout(deadline)
		if terr != nil {
			return nil, terr
		}
		subcalls++
		if subcalls > c.maxLogicalLookupSubcalls {
			return nil, protocol.ErrOverflow
		}
		var payload []byte
		if err := c.callWithRetry(func() error {
			_, callPayload, rerr := c.doRawCallWithTimeout(protocol.MethodAppsLookup, reqBuf[:reqLen], remainingTimeout)
			if rerr != nil {
				return rerr
			}
			payload = callPayload
			return nil
		}); err != nil {
			return nil, err
		}
		view, derr := protocol.DecodeAppsLookupResponse(payload)
		if derr != nil {
			return nil, derr
		}
		if !haveGeneration {
			generation = view.Generation
			haveGeneration = true
		} else if view.Generation != generation {
			return nil, protocol.ErrBadLayout
		}
		reqCountU32, err := checkedLookupU32(len(reqPids))
		if err != nil {
			return nil, err
		}
		if view.ItemCount != reqCountU32 {
			return nil, protocol.ErrBadItemCount
		}
		payloadExceededAt := -1
		for i, expected := range reqPids {
			itemIndex, ierr := checkedLookupU32(i)
			if ierr != nil {
				return nil, ierr
			}
			item, ierr := view.Item(itemIndex)
			if ierr != nil {
				return nil, ierr
			}
			if item.Pid != expected {
				return nil, protocol.ErrBadLayout
			}
			if item.Status == protocol.PidLookupPayloadExceeded {
				payloadExceededAt = i
				break
			}
			rawItem, ierr := view.RawItem(itemIndex)
			if ierr != nil {
				return nil, ierr
			}
			rawItems = append(rawItems, cloneLookupRawItem(rawItem))
		}
		if payloadExceededAt < 0 {
			start += reqCount
			if start < len(pids) {
				continue
			}
			rawItemCount, cerr := checkedLookupU32(len(rawItems))
			if cerr != nil {
				return nil, cerr
			}
			if rawItemCount != expectedCount {
				return nil, protocol.ErrBadItemCount
			}
			size, serr := lookupRawResponseSize(protocol.AppsLookupRespHdr, rawItems)
			if serr != nil {
				return nil, serr
			}
			if lookupSizeExceedsLimit(size, c.maxLogicalLookupResponseBytes) {
				return nil, protocol.ErrOverflow
			}
			stitched := make([]byte, size)
			n, eerr := protocol.EncodeAppsLookupRawResponse(rawItems, generation, stitched)
			if eerr != nil {
				return nil, eerr
			}
			return protocol.DecodeAppsLookupResponse(stitched[:n])
		}
		for i := payloadExceededAt; i < len(reqPids); i++ {
			itemIndex, ierr := checkedLookupU32(i)
			if ierr != nil {
				return nil, ierr
			}
			item, ierr := view.Item(itemIndex)
			if ierr != nil {
				return nil, ierr
			}
			if item.Pid != reqPids[i] || item.Status != protocol.PidLookupPayloadExceeded {
				return nil, protocol.ErrBadLayout
			}
		}
		if payloadExceededAt == 0 {
			return nil, protocol.ErrOverflow
		}
		start += payloadExceededAt
	}
}

// AppsLookupDispatch adapts a typed apps lookup handler to the raw dispatch shape.
func AppsLookupDispatch(handle AppsLookupHandler) DispatchHandler {
	if handle == nil {
		return nil
	}
	return func(request []byte, responseBuf []byte) (int, error) {
		n, err := protocol.DispatchAppsLookup(request, responseBuf, func(req *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
			return handle(req, builder)
		})
		if errors.Is(err, protocol.ErrHandlerFailed) {
			return 0, errHandlerFailed
		}
		return n, err
	}
}
