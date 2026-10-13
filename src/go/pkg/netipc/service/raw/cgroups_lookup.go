package raw

import (
	"bytes"
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// CgroupsLookupHandler serves a single CGROUPS_LOOKUP service kind.
type CgroupsLookupHandler func(*protocol.CgroupsLookupRequestView, *protocol.CgroupsLookupBuilder) bool

func cgroupsLookupRequestSize(paths [][]byte) (int, error) {
	return cgroupsLookupRequestSizeForCount(len(paths), func(i int) int {
		return len(paths[i])
	})
}

func cgroupsLookupRequestSizeForLengths(pathLens []int) (int, error) {
	return cgroupsLookupRequestSizeForCount(len(pathLens), func(i int) int {
		return pathLens[i]
	})
}

func cgroupsLookupRequestSizeForCount(count int, pathLenAt func(int) int) (int, error) {
	dirSize, err := checkedLookupMul(count, protocol.LookupDirEntrySize)
	if err != nil {
		return 0, err
	}
	size, err := checkedLookupAdd(protocol.CgroupsLookupReqHdr, dirSize)
	if err != nil {
		return 0, err
	}
	data := size
	for i := range count {
		data, err = checkedLookupAlign8(data)
		if err != nil {
			return 0, err
		}
		data, err = checkedLookupAdd(data, pathLenAt(i))
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

func cgroupsLookupNextRequest(paths [][]byte, maxPayload uint32) (int, int, error) {
	if len(paths) == 0 {
		size, err := cgroupsLookupRequestSize(nil)
		return 0, size, err
	}
	if maxPayload == 0 {
		maxPayload = protocol.MaxPayloadDefault
	}
	lo, hi := 1, len(paths)
	bestCount := 0
	bestSize := 0
	for lo <= hi {
		mid := lo + (hi-lo)/2
		size, err := cgroupsLookupRequestSize(paths[:mid])
		if err != nil {
			return 0, 0, err
		}
		if lookupSizeWithinLimit(size, maxPayload) {
			bestCount = mid
			bestSize = size
			lo = mid + 1
			continue
		}
		hi = mid - 1
	}
	if bestCount == 0 {
		return 0, 0, protocol.ErrOverflow
	}
	return bestCount, bestSize, nil
}

func cgroupsLookupOversizedRequestItem(path []byte) ([]byte, error) {
	size, err := cgroupsLookupOversizedRequestItemSize(len(path))
	if err != nil {
		return nil, err
	}

	buf := make([]byte, size)
	builder := protocol.NewCgroupsLookupBuilder(buf, 1, 0)
	if err := builder.Add(protocol.CgroupLookupOversizedItem, 0, path, nil, nil); err != nil {
		return nil, err
	}
	n := builder.Finish()
	view, err := protocol.DecodeCgroupsLookupResponse(buf[:n])
	if err != nil {
		return nil, err
	}
	rawItem, err := view.RawItem(0)
	if err != nil {
		return nil, err
	}
	return cloneLookupRawItem(rawItem), nil
}

func cgroupsLookupOversizedRequestItemSize(pathLen int) (int, error) {
	size, err := checkedLookupAdd(protocol.CgroupsLookupRespHdr, protocol.LookupDirEntrySize)
	if err != nil {
		return 0, err
	}
	size, err = checkedLookupAlign8(size)
	if err != nil {
		return 0, err
	}
	itemSize, err := checkedLookupAdd(protocol.CgroupsLookupItemHdr, pathLen)
	if err != nil {
		return 0, err
	}
	itemSize, err = checkedLookupAdd(itemSize, 2)
	if err != nil {
		return 0, err
	}
	return checkedLookupAdd(size, itemSize)
}

// CallCgroupsLookup performs a blocking typed CGROUPS_LOOKUP call.
// The returned view is valid until the next typed call on this client.
func (c *Client) CallCgroupsLookup(paths [][]byte) (*protocol.CgroupsLookupResponseView, error) {
	return c.CallCgroupsLookupWithTimeout(paths, 0)
}

func (c *Client) CallCgroupsLookupWithTimeout(paths [][]byte, timeoutMs uint32) (*protocol.CgroupsLookupResponseView, error) {
	if err := c.validateMethod(protocol.MethodCgroupsLookup); err != nil {
		return nil, err
	}
	if uint64(len(paths)) > uint64(c.maxLogicalLookupItems) {
		return nil, protocol.ErrOverflow
	}
	if err := c.ensureReadyForLogicalLookup(); err != nil {
		return nil, err
	}

	expectedCount, err := checkedLookupU32(len(paths))
	if err != nil {
		return nil, err
	}
	rawItems := make([][]byte, 0, len(paths))
	start := 0
	var generation uint64
	haveGeneration := false
	subcalls := uint32(0)
	deadline := c.newLookupDeadline(timeoutMs)
	for {
		reqCount, reqSize, err := cgroupsLookupNextRequest(paths[start:], c.sessionMaxRequestPayloadBytes())
		if err != nil {
			if err == protocol.ErrOverflow && start < len(paths) {
				oneItemSize, serr := cgroupsLookupRequestSize(paths[start : start+1])
				if serr == nil {
					if c.ensureLookupRequestCapacity(oneItemSize) == nil {
						continue
					}
				}
				rawItem, serr := cgroupsLookupOversizedRequestItem(paths[start])
				if serr != nil {
					return nil, serr
				}
				rawItems = append(rawItems, rawItem)
				start++
				continue
			}
			return nil, err
		}
		reqPaths := paths[start : start+reqCount]
		reqBuf := ensureClientScratch(&c.requestBuf, reqSize)
		reqLen, err := protocol.EncodeCgroupsLookupRequest(reqPaths, reqBuf)
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
			_, callPayload, rerr := c.doRawCallWithTimeout(protocol.MethodCgroupsLookup, reqBuf[:reqLen], remainingTimeout)
			if rerr != nil {
				return rerr
			}
			payload = callPayload
			return nil
		}); err != nil {
			return nil, err
		}
		view, derr := protocol.DecodeCgroupsLookupResponse(payload)
		if derr != nil {
			return nil, derr
		}
		if !haveGeneration {
			generation = view.Generation
			haveGeneration = true
		} else if view.Generation != generation {
			return nil, protocol.ErrBadLayout
		}
		reqCountU32, err := checkedLookupU32(len(reqPaths))
		if err != nil {
			return nil, err
		}
		if view.ItemCount != reqCountU32 {
			return nil, protocol.ErrBadItemCount
		}
		payloadExceededAt := -1
		for i, expected := range reqPaths {
			itemIndex, ierr := checkedLookupU32(i)
			if ierr != nil {
				return nil, ierr
			}
			item, ierr := view.Item(itemIndex)
			if ierr != nil {
				return nil, ierr
			}
			if !bytes.Equal(item.Path.Bytes(), expected) {
				return nil, protocol.ErrBadLayout
			}
			if item.Status == protocol.CgroupLookupPayloadExceeded {
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
			if start < len(paths) {
				continue
			}
			rawItemCount, cerr := checkedLookupU32(len(rawItems))
			if cerr != nil {
				return nil, cerr
			}
			if rawItemCount != expectedCount {
				return nil, protocol.ErrBadItemCount
			}
			size, serr := lookupRawResponseSize(protocol.CgroupsLookupRespHdr, rawItems)
			if serr != nil {
				return nil, serr
			}
			if lookupSizeExceedsLimit(size, c.maxLogicalLookupResponseBytes) {
				return nil, protocol.ErrOverflow
			}
			stitched := make([]byte, size)
			n, eerr := protocol.EncodeCgroupsLookupRawResponse(rawItems, generation, stitched)
			if eerr != nil {
				return nil, eerr
			}
			return protocol.DecodeCgroupsLookupResponse(stitched[:n])
		}
		for i := payloadExceededAt; i < len(reqPaths); i++ {
			itemIndex, ierr := checkedLookupU32(i)
			if ierr != nil {
				return nil, ierr
			}
			item, ierr := view.Item(itemIndex)
			if ierr != nil {
				return nil, ierr
			}
			if !bytes.Equal(item.Path.Bytes(), reqPaths[i]) ||
				item.Status != protocol.CgroupLookupPayloadExceeded {
				return nil, protocol.ErrBadLayout
			}
		}
		if payloadExceededAt == 0 {
			return nil, protocol.ErrOverflow
		}
		start += payloadExceededAt
	}
}

// CgroupsLookupDispatch adapts a typed cgroups lookup handler to the raw dispatch shape.
func CgroupsLookupDispatch(handle CgroupsLookupHandler) DispatchHandler {
	if handle == nil {
		return nil
	}
	return func(request []byte, responseBuf []byte) (int, error) {
		n, err := protocol.DispatchCgroupsLookup(request, responseBuf, func(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
			return handle(req, builder)
		})
		if errors.Is(err, protocol.ErrHandlerFailed) {
			return 0, errHandlerFailed
		}
		return n, err
	}
}
