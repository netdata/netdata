package raw

import "github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"

// StringReverseHandler serves a single STRING_REVERSE service kind.
type StringReverseHandler func(string) (string, bool)

// CallStringReverse performs a blocking STRING_REVERSE call.
// The returned view is valid until the next typed call on this client.
func (c *Client) CallStringReverse(requestStr string) (*protocol.StringReverseView, error) {
	return c.CallStringReverseWithTimeout(requestStr, 0)
}

func (c *Client) CallStringReverseWithTimeout(requestStr string, timeoutMs uint32) (*protocol.StringReverseView, error) {
	if err := c.validateMethod(protocol.MethodStringReverse); err != nil {
		return nil, err
	}

	var result *protocol.StringReverseView

	err := c.callWithRetry(func() error {
		reqBuf := ensureClientScratch(&c.requestBuf, protocol.StringReverseHdrSize+len(requestStr)+1)
		if protocol.StringReverseEncode(requestStr, reqBuf) == 0 {
			return protocol.ErrTruncated
		}

		_, payload, rerr := c.doRawCallWithTimeout(protocol.MethodStringReverse, reqBuf, timeoutMs)
		if rerr != nil {
			return rerr
		}

		view, derr := protocol.StringReverseDecode(payload)
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

// StringReverseDispatch adapts a typed string-reverse handler to the raw dispatch shape.
func StringReverseDispatch(handle StringReverseHandler) DispatchHandler {
	if handle == nil {
		return nil
	}
	return func(request []byte, responseBuf []byte) (int, error) {
		view, err := protocol.StringReverseDecode(request)
		if err != nil {
			return 0, err
		}
		result, ok := handle(view.Str)
		if !ok {
			return 0, errHandlerFailed
		}
		n := protocol.StringReverseEncode(result, responseBuf)
		if n == 0 {
			return 0, protocol.ErrOverflow
		}
		return n, nil
	}
}
