// Package raw provides internal L2/L3 helpers used by the protocol, fixtures,
// and benchmarks.
//
// Pure composition of L1 transport + Codec. No direct socket/pipe calls.
// Client manages connection lifecycle with at-least-once retry.
// Server handles accept, read, dispatch, respond.
//
// Pure Go — no cgo. Works with CGO_ENABLED=0.
package raw

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// Poll/receive timeout for server loops (ms). Controls shutdown detection latency.
const serverPollTimeoutMs = 100

// ---------------------------------------------------------------------------
//  Client state (shared across platforms)
// ---------------------------------------------------------------------------

// ClientState represents the connection state machine.
type ClientState int

const (
	StateDisconnected ClientState = iota
	StateConnecting
	StateReady
	StateNotFound
	StateAuthFailed
	StateIncompatible
	StateBroken
)

// ClientStatus is a diagnostic counters snapshot.
type ClientStatus struct {
	State          ClientState
	ConnectCount   uint32
	ReconnectCount uint32
	CallCount      uint32
	ErrorCount     uint32
}

// IncrementHandler serves a single INCREMENT service kind.
type IncrementHandler func(uint64) (uint64, bool)

// StringReverseHandler serves a single STRING_REVERSE service kind.
type StringReverseHandler func(string) (string, bool)

// SnapshotHandler serves a single CGROUPS_SNAPSHOT service kind.
type SnapshotHandler func(*protocol.CgroupsRequest, *protocol.CgroupsBuilder) bool

var errHandlerFailed = errors.New("dispatch handler failed")

// DispatchHandler validates/decodes a single service kind request and writes
// the matching response into responseBuf.
type DispatchHandler func(request []byte, responseBuf []byte) (int, error)

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

// SnapshotMaxItems returns the item budget for a single snapshot service kind.
func SnapshotMaxItems(responseBufSize int, override uint32) uint32 {
	if override != 0 {
		return override
	}
	return protocol.EstimateCgroupsMaxItems(responseBufSize)
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

// ---------------------------------------------------------------------------
//  L3 cache types (shared across platforms)
// ---------------------------------------------------------------------------

// Default response buffer size for L3 cache refresh.
const cacheResponseBufSize = 65536

// CacheItem is an owned copy of a single cgroup item.
// Built from ephemeral L2 views during cache construction.
type CacheItem struct {
	Hash    uint32
	Options uint32
	Enabled uint32
	Name    string // owned copy
	Path    string // owned copy
}

// CacheStatus is a diagnostic snapshot for the L3 cache.
type CacheStatus struct {
	Populated           bool
	ItemCount           uint32
	SystemdEnabled      uint32
	Generation          uint64
	RefreshSuccessCount uint32
	RefreshFailureCount uint32
	ConnectionState     ClientState // underlying L2 client state
	LastRefreshTs       int64       // monotonic timestamp (ms) of last successful refresh, 0 if never
}
