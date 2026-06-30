package raw

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

const (
	LookupLogicalItemsDefault         uint32 = 65536
	LookupLogicalSubcallsDefault      uint32 = 4096
	LookupLogicalResponseBytesDefault uint32 = 64 * 1024 * 1024
)

type LookupLogicalConfig struct {
	MaxItems         uint32
	MaxSubcalls      uint32
	MaxResponseBytes uint32
}

func normalizeLookupLogicalConfig(config LookupLogicalConfig) LookupLogicalConfig {
	if config.MaxItems == 0 {
		config.MaxItems = LookupLogicalItemsDefault
	}
	if config.MaxSubcalls == 0 {
		config.MaxSubcalls = LookupLogicalSubcallsDefault
	}
	if config.MaxResponseBytes == 0 {
		config.MaxResponseBytes = LookupLogicalResponseBytesDefault
	}
	return config
}

func (c *Client) SetLookupLogicalConfig(config LookupLogicalConfig) {
	config = normalizeLookupLogicalConfig(config)
	c.maxLogicalLookupItems = config.MaxItems
	c.maxLogicalLookupSubcalls = config.MaxSubcalls
	c.maxLogicalLookupResponseBytes = config.MaxResponseBytes
}

func (c *Client) ensureReadyForLogicalLookup() error {
	if c.state != StateReady {
		c.errorCount++
		return protocol.ErrBadLayout
	}
	if c.abortRequested.Load() {
		c.disconnect()
		c.state = StateBroken
		c.errorCount++
		return protocol.ErrAborted
	}
	return nil
}

func (c *Client) ensureLookupRequestCapacity(required int) error {
	if c.abortRequested.Load() {
		c.disconnect()
		c.state = StateBroken
		c.errorCount++
		return protocol.ErrAborted
	}
	if required < 0 || uint64(required) > uint64(^uint32(0)) {
		return protocol.ErrOverflow
	}
	maxRequestPayloadBytes := c.config.MaxRequestPayloadBytes
	if maxRequestPayloadBytes == 0 {
		maxRequestPayloadBytes = protocol.MaxPayloadDefault
	}
	if uint32(required) > maxRequestPayloadBytes {
		return protocol.ErrOverflow
	}
	if c.sessionMaxRequestPayloadBytes() >= uint32(required) {
		return nil
	}
	prevReq := c.sessionMaxRequestPayloadBytes()
	c.disconnect()
	c.state = StateBroken
	c.state = c.tryConnect()
	if c.state != StateReady {
		c.errorCount++
		return protocol.ErrOverflow
	}
	c.reconnectCount++
	if c.sessionMaxRequestPayloadBytes() <= prevReq ||
		c.sessionMaxRequestPayloadBytes() < uint32(required) {
		c.disconnect()
		c.state = StateBroken
		c.errorCount++
		return protocol.ErrOverflow
	}
	return nil
}

func (c *Client) newLookupDeadline(timeoutMs uint32) time.Time {
	return time.Now().Add(time.Duration(c.resolvedCallTimeout(timeoutMs)) * time.Millisecond)
}

func (c *Client) lookupRemainingTimeout(deadline time.Time) (uint32, error) {
	if c.abortRequested.Load() {
		return 0, protocol.ErrAborted
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return 0, protocol.ErrTimeout
	}
	ms := remaining.Milliseconds()
	if ms <= 0 {
		return 1, nil
	}
	if ms > int64(^uint32(0)) {
		return ^uint32(0), nil
	}
	return uint32(ms), nil // #nosec G115 -- ms is bounded above.
}

func checkedLookupAdd(a, b int) (int, error) {
	if a < 0 || b < 0 {
		return 0, protocol.ErrOverflow
	}
	maxInt := int(^uint(0) >> 1)
	if a > maxInt-b {
		return 0, protocol.ErrOverflow
	}
	return a + b, nil
}

func checkedLookupMul(a, b int) (int, error) {
	if a < 0 || b < 0 {
		return 0, protocol.ErrOverflow
	}
	maxInt := int(^uint(0) >> 1)
	if a != 0 && b > maxInt/a {
		return 0, protocol.ErrOverflow
	}
	return a * b, nil
}

func checkedLookupAlign8(v int) (int, error) {
	if v < 0 {
		return 0, protocol.ErrOverflow
	}
	maxInt := int(^uint(0) >> 1)
	if v > maxInt-7 {
		return 0, protocol.ErrOverflow
	}
	return protocol.Align8(v), nil
}

func checkedLookupU32(value int) (uint32, error) {
	if value < 0 || uint64(value) > uint64(^uint32(0)) {
		return 0, protocol.ErrOverflow
	}
	return uint32(value), nil // #nosec G115 -- value is bounded by the uint32 maximum above.
}

func cloneLookupRawItem(item []byte) []byte {
	cloned := make([]byte, len(item))
	copy(cloned, item)
	return cloned
}

func lookupRawResponseSize(hdrSize int, items [][]byte) (int, error) {
	return lookupRawResponseSizeForCount(hdrSize, len(items), func(i int) int {
		return len(items[i])
	})
}

func lookupRawResponseSizeForLengths(hdrSize int, itemLens []int) (int, error) {
	return lookupRawResponseSizeForCount(hdrSize, len(itemLens), func(i int) int {
		return itemLens[i]
	})
}

func lookupRawResponseSizeForCount(hdrSize, count int, itemLenAt func(int) int) (int, error) {
	dirSize, err := checkedLookupMul(count, protocol.LookupDirEntrySize)
	if err != nil {
		return 0, err
	}
	data, err := checkedLookupAdd(hdrSize, dirSize)
	if err != nil {
		return 0, err
	}
	for i := range count {
		data, err = checkedLookupAlign8(data)
		if err != nil {
			return 0, err
		}
		data, err = checkedLookupAdd(data, itemLenAt(i))
		if err != nil {
			return 0, err
		}
	}
	return data, nil
}

func lookupSizeWithinLimit(size int, limit uint32) bool {
	size32, err := checkedLookupU32(size)
	return err == nil && size32 <= limit
}

func lookupSizeExceedsLimit(size int, limit uint32) bool {
	size32, err := checkedLookupU32(size)
	return err != nil || size32 > limit
}
