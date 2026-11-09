package raw

import (
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// SnapshotHandler serves a single CGROUPS_SNAPSHOT service kind. The request
// is decoded into scalar fields, and the builder writes into the response
// buffer, which is valid only during the call.
type SnapshotHandler func(*protocol.CgroupsRequest, *protocol.CgroupsBuilder) bool

// SnapshotMaxItems returns the item budget for a single snapshot service kind.
func SnapshotMaxItems(responseBufSize int, override uint32) uint32 {
	if override != 0 {
		return override
	}
	return protocol.EstimateCgroupsMaxItems(responseBufSize)
}

// CallSnapshot performs a blocking typed cgroups snapshot call.
// The returned view is valid until the next typed call on this client.
func (c *Client) CallSnapshot() (*protocol.CgroupsResponseView, error) {
	return c.CallSnapshotWithTimeout(0)
}

func (c *Client) CallSnapshotWithTimeout(timeoutMs uint32) (*protocol.CgroupsResponseView, error) {
	if err := c.validateMethod(protocol.MethodCgroupsSnapshot); err != nil {
		return nil, err
	}

	err := c.callWithRetry(timeoutMs, func() error {
		req := protocol.CgroupsRequest{LayoutVersion: 1, Flags: 0}
		// Client scratch: the payload reaches the transport, so a local
		// array would be a heap allocation per call.
		reqBuf := ensureClientScratch(&c.requestBuf, 4)
		if req.Encode(reqBuf) == 0 {
			return protocol.ErrTruncated
		}

		_, payload, rerr := c.doRawCallWithTimeout(protocol.MethodCgroupsSnapshot, reqBuf, timeoutMs)
		if rerr != nil {
			return rerr
		}

		view, derr := protocol.DecodeCgroupsResponse(payload)
		if derr != nil {
			return derr
		}
		c.snapshotView = view
		return nil
	})
	if err != nil {
		return nil, err
	}
	// The view lives in the client, so returning it does not allocate.
	return &c.snapshotView, nil
}

// The request and builder handed to a snapshot handler escape through that
// call; pooled scratch keeps them off the per-request heap. Each dispatch
// owns its entry until it returns.
type snapshotScratch struct {
	req     protocol.CgroupsRequest
	builder protocol.CgroupsBuilder
}

var snapshotScratchPool = sync.Pool{New: func() any { return new(snapshotScratch) }}

// release drops the reference to the caller's response buffer.
func (s *snapshotScratch) release() {
	*s = snapshotScratch{}
	snapshotScratchPool.Put(s)
}

// SnapshotDispatch adapts a typed snapshot handler to the raw dispatch shape.
func SnapshotDispatch(handle SnapshotHandler, maxItems uint32) DispatchHandler {
	if handle == nil {
		return nil
	}
	return func(request []byte, responseBuf []byte) (int, error) {
		scratch := snapshotScratchPool.Get().(*snapshotScratch)
		defer scratch.release()
		var err error
		scratch.req, err = protocol.DecodeCgroupsRequest(request)
		if err != nil {
			return 0, err
		}
		itemBudget := SnapshotMaxItems(len(responseBuf), maxItems)
		if itemBudget == 0 {
			return 0, protocol.ErrOverflow
		}
		minRequired, ok := protocol.CgroupsBuilderMinBytes(itemBudget)
		if !ok || len(responseBuf) < minRequired {
			return 0, protocol.ErrOverflow
		}
		builder := &scratch.builder
		builder.Reset(responseBuf, itemBudget, 0, 0)
		if !handle(&scratch.req, builder) {
			return 0, errHandlerFailed
		}
		n := builder.Finish()
		if n == 0 {
			return 0, protocol.ErrOverflow
		}
		return n, nil
	}
}
