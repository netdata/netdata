package raw

import "github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"

// SnapshotHandler serves a single CGROUPS_SNAPSHOT service kind.
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

	var result *protocol.CgroupsResponseView

	err := c.callWithRetry(func() error {
		req := protocol.CgroupsRequest{LayoutVersion: 1, Flags: 0}
		var reqBuf [4]byte
		if req.Encode(reqBuf[:]) == 0 {
			return protocol.ErrTruncated
		}

		_, payload, rerr := c.doRawCallWithTimeout(protocol.MethodCgroupsSnapshot, reqBuf[:], timeoutMs)
		if rerr != nil {
			return rerr
		}

		view, derr := protocol.DecodeCgroupsResponse(payload)
		if derr != nil {
			return derr
		}
		result = &view
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SnapshotDispatch adapts a typed snapshot handler to the raw dispatch shape.
func SnapshotDispatch(handle SnapshotHandler, maxItems uint32) DispatchHandler {
	if handle == nil {
		return nil
	}
	return func(request []byte, responseBuf []byte) (int, error) {
		req, err := protocol.DecodeCgroupsRequest(request)
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
		builder := protocol.NewCgroupsBuilder(responseBuf, itemBudget, 0, 0)
		if !handle(&req, builder) {
			return 0, errHandlerFailed
		}
		n := builder.Finish()
		if n == 0 {
			return 0, protocol.ErrOverflow
		}
		return n, nil
	}
}
