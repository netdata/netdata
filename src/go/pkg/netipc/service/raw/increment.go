package raw

import "github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"

// IncrementHandler serves a single INCREMENT service kind.
type IncrementHandler func(uint64) (uint64, bool)

// CallIncrement performs a blocking INCREMENT call.
// Sends requestValue, returns the server's response value.
func (c *Client) CallIncrement(requestValue uint64) (uint64, error) {
	return c.CallIncrementWithTimeout(requestValue, 0)
}

func (c *Client) CallIncrementWithTimeout(requestValue uint64, timeoutMs uint32) (uint64, error) {
	if err := c.validateMethod(protocol.MethodIncrement); err != nil {
		return 0, err
	}

	var result uint64

	err := c.callWithRetry(func() error {
		var reqBuf [protocol.IncrementPayloadSize]byte
		if protocol.IncrementEncode(requestValue, reqBuf[:]) == 0 {
			return protocol.ErrTruncated
		}

		_, payload, rerr := c.doRawCallWithTimeout(protocol.MethodIncrement, reqBuf[:], timeoutMs)
		if rerr != nil {
			return rerr
		}

		val, derr := protocol.IncrementDecode(payload)
		if derr != nil {
			return derr
		}
		result = val
		return nil
	})
	return result, err
}

// CallIncrementBatch performs a blocking batch INCREMENT call.
// Sends multiple values, returns the server's response values.
func (c *Client) CallIncrementBatch(values []uint64) ([]uint64, error) {
	return c.CallIncrementBatchWithTimeout(values, 0)
}

func (c *Client) CallIncrementBatchWithTimeout(values []uint64, timeoutMs uint32) ([]uint64, error) {
	if err := c.validateMethod(protocol.MethodIncrement); err != nil {
		return nil, err
	}

	if len(values) == 0 {
		return nil, nil
	}

	var results []uint64
	itemCount, err := checkedLookupU32(len(values))
	if err != nil {
		return nil, err
	}

	err = c.callWithRetry(func() error {
		batchBufSize, err := incrementBatchRequestSize(len(values))
		if err != nil {
			return err
		}
		batchBuf := ensureClientScratch(&c.requestBuf, batchBufSize)
		bb := protocol.NewBatchBuilder(batchBuf, itemCount)

		for _, v := range values {
			var item [protocol.IncrementPayloadSize]byte
			if protocol.IncrementEncode(v, item[:]) == 0 {
				return protocol.ErrTruncated
			}
			if err := bb.Add(item[:]); err != nil {
				return err
			}
		}

		totalPayloadLen, _ := bb.Finish()
		reqPayload := batchBuf[:totalPayloadLen]

		hdr := protocol.Header{
			Kind:            protocol.KindRequest,
			Code:            protocol.MethodIncrement,
			Flags:           protocol.FlagBatch,
			ItemCount:       itemCount,
			MessageID:       uint64(c.callCount) + 1,
			TransportStatus: protocol.StatusOK,
		}

		if err := c.transportSend(&hdr, reqPayload); err != nil {
			return err
		}

		respHdr, respPayload, err := c.transportReceiveWithControl(
			c.resolvedCallTimeout(timeoutMs), c.abortSignal())
		if err != nil {
			return err
		}

		if respHdr.Kind != protocol.KindResponse {
			return protocol.ErrBadKind
		}
		if respHdr.Code != protocol.MethodIncrement {
			return protocol.ErrBadLayout
		}
		if respHdr.MessageID != hdr.MessageID {
			return protocol.ErrBadLayout
		}
		switch respHdr.TransportStatus {
		case protocol.StatusOK:
		case protocol.StatusLimitExceeded:
			if current := c.sessionMaxResponsePayloadBytes(); current > 0 {
				if current >= ^uint32(0)/2 {
					c.noteResponseCapacity(^uint32(0))
				} else {
					c.noteResponseCapacity(current * 2)
				}
			}
			return protocol.ErrOverflow
		default:
			return protocol.ErrBadLayout
		}
		if respHdr.Flags&protocol.FlagBatch == 0 || respHdr.ItemCount != itemCount {
			return protocol.ErrBadItemCount
		}

		out := make([]uint64, itemCount)
		for i := range itemCount {
			itemData, gerr := protocol.BatchItemGet(respPayload, itemCount, i)
			if gerr != nil {
				return gerr
			}
			val, derr := protocol.IncrementDecode(itemData)
			if derr != nil {
				return derr
			}
			out[i] = val
		}
		results = out
		return nil
	})
	return results, err
}

func incrementBatchRequestSize(valueCount int) (int, error) {
	if _, err := checkedLookupU32(valueCount); err != nil {
		return 0, err
	}
	dirBytes, err := checkedLookupMul(valueCount, 8)
	if err != nil {
		return 0, err
	}
	dirAligned, err := checkedLookupAlign8(dirBytes)
	if err != nil {
		return 0, err
	}
	itemsBytes, err := checkedLookupMul(valueCount, protocol.IncrementPayloadSize)
	if err != nil {
		return 0, err
	}
	paddingBytes, err := checkedLookupMul(valueCount, protocol.Alignment)
	if err != nil {
		return 0, err
	}
	batchBufSize, err := checkedLookupAdd(dirAligned, itemsBytes)
	if err != nil {
		return 0, err
	}
	return checkedLookupAdd(batchBufSize, paddingBytes)
}

// IncrementDispatch adapts a typed increment handler to the raw dispatch shape.
func IncrementDispatch(handle IncrementHandler) DispatchHandler {
	if handle == nil {
		return nil
	}
	return func(request []byte, responseBuf []byte) (int, error) {
		value, err := protocol.IncrementDecode(request)
		if err != nil {
			return 0, err
		}
		result, ok := handle(value)
		if !ok {
			return 0, errHandlerFailed
		}
		n := protocol.IncrementEncode(result, responseBuf)
		if n == 0 {
			return 0, protocol.ErrOverflow
		}
		return n, nil
	}
}
