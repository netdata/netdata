package raw

import (
	"encoding/binary"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func patchLookupResponseItemU16(t *testing.T, payload []byte, hdrSize int, itemCount uint32, index uint32, itemOffset int, value uint16) {
	t.Helper()

	dir := hdrSize + int(index)*protocol.LookupDirEntrySize
	if dir+protocol.LookupDirEntrySize > len(payload) {
		t.Fatalf("lookup response dir out of bounds: dir=%d len=%d", dir, len(payload))
	}
	packedStart := hdrSize + int(itemCount)*protocol.LookupDirEntrySize
	itemOff := int(binary.NativeEndian.Uint32(payload[dir : dir+4]))
	field := packedStart + itemOff + itemOffset
	if field+2 > len(payload) {
		t.Fatalf("lookup response field out of bounds: field=%d len=%d", field, len(payload))
	}
	binary.NativeEndian.PutUint16(payload[field:field+2], value)
}

func TestLookupLogicalConfigDefaults(t *testing.T) {
	got := normalizeLookupLogicalConfig(LookupLogicalConfig{})
	if got.MaxItems != LookupLogicalItemsDefault ||
		got.MaxSubcalls != LookupLogicalSubcallsDefault ||
		got.MaxResponseBytes != LookupLogicalResponseBytesDefault {
		t.Fatalf("default logical config = %+v", got)
	}

	got = normalizeLookupLogicalConfig(LookupLogicalConfig{MaxItems: 1, MaxSubcalls: 2, MaxResponseBytes: 3})
	if got.MaxItems != 1 || got.MaxSubcalls != 2 || got.MaxResponseBytes != 3 {
		t.Fatalf("explicit logical config = %+v", got)
	}
}

func TestLookupCommonSizingHelpers(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	if _, err := checkedLookupAdd(-1, 1); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("checkedLookupAdd negative error = %v", err)
	}
	if _, err := checkedLookupAdd(maxInt, 1); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("checkedLookupAdd overflow error = %v", err)
	}
	if _, err := checkedLookupMul(-1, 1); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("checkedLookupMul negative error = %v", err)
	}
	if _, err := checkedLookupMul(maxInt, 2); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("checkedLookupMul overflow error = %v", err)
	}
	if _, err := checkedLookupAlign8(-1); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("checkedLookupAlign8 negative error = %v", err)
	}
	if _, err := checkedLookupAlign8(maxInt - 6); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("checkedLookupAlign8 overflow error = %v", err)
	}
	if _, err := checkedLookupU32(-1); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("checkedLookupU32 negative error = %v", err)
	}
	if _, err := checkedLookupU32(int(uint64(^uint32(0)) + 1)); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("checkedLookupU32 overflow error = %v", err)
	}
}

func TestLookupNextRequestSizingEdges(t *testing.T) {
	appsCount, appsSize, err := appsLookupNextRequest(nil, 0)
	if err != nil {
		t.Fatalf("apps zero-item next request: %v", err)
	}
	if appsCount != 0 || appsSize != protocol.AppsLookupReqHdr {
		t.Fatalf("apps zero-item next request = %d/%d, want 0/%d",
			appsCount, appsSize, protocol.AppsLookupReqHdr)
	}

	appsCount, appsSize, err = appsLookupNextRequest([]uint32{1, 2}, 0)
	if err != nil {
		t.Fatalf("apps default-cap next request: %v", err)
	}
	if appsCount != 2 || appsSize <= protocol.AppsLookupReqHdr {
		t.Fatalf("apps default-cap next request = %d/%d, want both items and body",
			appsCount, appsSize)
	}

	appsOneItem := protocol.AppsLookupReqHdr + protocol.LookupDirEntrySize + protocol.AppsLookupKeySize
	if _, _, err := appsLookupNextRequest([]uint32{1}, uint32(appsOneItem-1)); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("apps too-small next request error = %v, want ErrOverflow", err)
	}

	cgroupsCount, cgroupsSize, err := cgroupsLookupNextRequest(nil, 0)
	if err != nil {
		t.Fatalf("cgroups zero-item next request: %v", err)
	}
	if cgroupsCount != 0 || cgroupsSize != protocol.CgroupsLookupReqHdr {
		t.Fatalf("cgroups zero-item next request = %d/%d, want 0/%d",
			cgroupsCount, cgroupsSize, protocol.CgroupsLookupReqHdr)
	}

	cgroupsCount, cgroupsSize, err = cgroupsLookupNextRequest([][]byte{[]byte("a"), []byte("b")}, 0)
	if err != nil {
		t.Fatalf("cgroups default-cap next request: %v", err)
	}
	if cgroupsCount != 2 || cgroupsSize <= protocol.CgroupsLookupReqHdr {
		t.Fatalf("cgroups default-cap next request = %d/%d, want both items and body",
			cgroupsCount, cgroupsSize)
	}

	if _, _, err := cgroupsLookupNextRequest(
		[][]byte{[]byte("abc")},
		uint32(protocol.CgroupsLookupReqHdr+protocol.LookupDirEntrySize),
	); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("cgroups too-small next request error = %v, want ErrOverflow", err)
	}
}

func TestLookupRawResponseSizeAndClone(t *testing.T) {
	item := []byte{1, 2, 3}
	cloned := cloneLookupRawItem(item)
	cloned[0] = 9
	if item[0] != 1 {
		t.Fatalf("clone changed source item")
	}

	size, err := lookupRawResponseSize(protocol.CgroupsLookupRespHdr, [][]byte{item, item})
	if err != nil {
		t.Fatalf("lookupRawResponseSize: %v", err)
	}
	if size <= protocol.CgroupsLookupRespHdr+2*protocol.LookupDirEntrySize {
		t.Fatalf("lookupRawResponseSize = %d, want room for payload", size)
	}
	if _, err := lookupRawResponseSize(-1, [][]byte{item}); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("lookupRawResponseSize negative header error = %v", err)
	}
	if _, err := lookupRawResponseSize(int(^uint(0)>>1), [][]byte{item}); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("lookupRawResponseSize overflowing header error = %v", err)
	}
}

func TestLookupSyntheticOverflowSizingGuards(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("synthetic oversized slice guards require 64-bit int")
	}
	maxInt := int(^uint(0) >> 1)

	hugePidCount := maxInt/protocol.LookupDirEntrySize + 1
	if _, err := appsLookupRequestSizeForCount(hugePidCount); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("apps huge request size error = %v, want ErrOverflow", err)
	}
	if _, err := appsLookupRequestSizeForCount(maxInt / protocol.LookupDirEntrySize); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("apps request header-size overflow error = %v, want ErrOverflow", err)
	}

	hugePathLen := maxInt
	if _, err := cgroupsLookupRequestSizeForLengths([]int{hugePathLen}); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("cgroups huge request size error = %v, want ErrOverflow", err)
	}
	if _, err := cgroupsLookupOversizedRequestItemSize(hugePathLen); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("cgroups huge oversized-item synthesis error = %v, want ErrOverflow", err)
	}
	if _, err := lookupRawResponseSizeForLengths(protocol.CgroupsLookupRespHdr, []int{hugePathLen}); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("huge raw response item size error = %v, want ErrOverflow", err)
	}

	cgroupsOneItemBase := protocol.CgroupsLookupReqHdr + protocol.LookupDirEntrySize
	addOneOverflowPathLen := maxInt - cgroupsOneItemBase
	if _, err := cgroupsLookupRequestSizeForLengths([]int{addOneOverflowPathLen}); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("cgroups request NUL-size overflow error = %v, want ErrOverflow", err)
	}
	cgroupsTwoItemBase := protocol.CgroupsLookupReqHdr + 2*protocol.LookupDirEntrySize
	alignOverflowPathLen := maxInt - cgroupsTwoItemBase - 4
	if _, err := cgroupsLookupRequestSizeForLengths([]int{alignOverflowPathLen, len("x")}); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("cgroups request next-item align overflow error = %v, want ErrOverflow", err)
	}

	if _, err := lookupRawResponseSizeForCount(protocol.CgroupsLookupRespHdr, hugePidCount, func(int) int {
		return 0
	}); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("huge raw response item count error = %v, want ErrOverflow", err)
	}

	if _, err := appsLookupRequestSizeForCount(maxInt/protocol.AppsLookupKeySize + 1); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("apps request key-size overflow error = %v, want ErrOverflow", err)
	}
	if _, err := cgroupsLookupRequestSizeForCount(-1, func(int) int { return 0 }); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("cgroups negative item count error = %v, want ErrOverflow", err)
	}
	if _, err := cgroupsLookupRequestSizeForLengths([]int{-1}); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("cgroups negative path length error = %v, want ErrOverflow", err)
	}
	if _, err := cgroupsLookupRequestSizeForCount(1, func(int) int { return maxInt }); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("cgroups path-size overflow error = %v, want ErrOverflow", err)
	}
	if _, err := cgroupsLookupOversizedRequestItemSize(-1); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("cgroups negative oversized item path error = %v, want ErrOverflow", err)
	}
	if _, err := cgroupsLookupOversizedRequestItemSize(maxInt); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("cgroups oversized item path-size overflow error = %v, want ErrOverflow", err)
	}
	if _, err := lookupRawResponseSizeForCount(-1, 1, func(int) int { return 0 }); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("negative raw response header error = %v, want ErrOverflow", err)
	}
	if _, err := lookupRawResponseSizeForCount(protocol.CgroupsLookupRespHdr, -1, func(int) int { return 0 }); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("negative raw response item count error = %v, want ErrOverflow", err)
	}
	if _, err := lookupRawResponseSizeForCount(protocol.CgroupsLookupRespHdr, 1, func(int) int { return -1 }); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("negative raw response item length error = %v, want ErrOverflow", err)
	}
	if lookupSizeWithinLimit(-1, 1) {
		t.Fatal("negative lookup size should not fit")
	}
	if !lookupSizeExceedsLimit(-1, 1) {
		t.Fatal("negative lookup size should exceed limit")
	}
	if _, err := incrementBatchRequestSize(-1); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("negative increment batch size error = %v, want ErrOverflow", err)
	}
}

func TestLookupDeadlineAndTimeoutHelpers(t *testing.T) {
	client := &Client{abortCh: make(chan struct{})}
	client.SetCallTimeout(5)
	deadline := client.newLookupDeadline(1)
	if remaining, err := client.lookupRemainingTimeout(deadline); err != nil || remaining == 0 {
		t.Fatalf("lookupRemainingTimeout active = %d/%v", remaining, err)
	}
	farFuture := time.Now().Add(time.Duration(^uint32(0)) * time.Millisecond).Add(time.Hour)
	if remaining, err := client.lookupRemainingTimeout(farFuture); err != nil || remaining != ^uint32(0) {
		t.Fatalf("lookupRemainingTimeout far future = %d/%v, want max uint32/nil", remaining, err)
	}
	if _, err := client.lookupRemainingTimeout(time.Now().Add(-time.Millisecond)); !errors.Is(err, protocol.ErrTimeout) {
		t.Fatalf("lookupRemainingTimeout expired error = %v", err)
	}
	client.Abort()
	if _, err := client.lookupRemainingTimeout(time.Now().Add(time.Second)); !errors.Is(err, protocol.ErrAborted) {
		t.Fatalf("lookupRemainingTimeout aborted error = %v", err)
	}
}

func TestClientTimeoutAbortCapacityAndRetryHelpers(t *testing.T) {
	client := &Client{abortCh: make(chan struct{})}
	if got := client.resolvedCallTimeout(7); got != 7 {
		t.Fatalf("explicit timeout = %d, want 7", got)
	}
	if got := client.resolvedCallTimeout(0); got != ClientCallTimeoutDefaultMs {
		t.Fatalf("default timeout = %d, want %d", got, ClientCallTimeoutDefaultMs)
	}
	client.SetCallTimeout(9)
	if got := client.resolvedCallTimeout(0); got != 9 {
		t.Fatalf("configured timeout = %d, want 9", got)
	}
	client.SetCallTimeout(0)
	if got := client.callTimeoutMs; got != ClientCallTimeoutDefaultMs {
		t.Fatalf("reset timeout = %d, want %d", got, ClientCallTimeoutDefaultMs)
	}

	client.Abort()
	client.Abort()
	select {
	case <-client.abortSignal():
	default:
		t.Fatal("abort signal should be closed after Abort")
	}
	client.ClearAbort()
	client.ClearAbort()
	select {
	case <-client.abortSignal():
		t.Fatal("abort signal should be open after ClearAbort")
	default:
	}

	client.noteResponseCapacity(33)
	if client.config.MaxResponsePayloadBytes != 64 {
		t.Fatalf("response capacity = %d, want 64", client.config.MaxResponsePayloadBytes)
	}
	client.noteResponseCapacity(32)
	if client.config.MaxResponsePayloadBytes != 64 {
		t.Fatalf("response capacity shrank to %d", client.config.MaxResponsePayloadBytes)
	}
	client.noteResponseCapacity(protocol.MaxPayloadCap + 1)
	if client.config.MaxResponsePayloadBytes != protocol.MaxPayloadCap {
		t.Fatalf("response capacity cap = %d, want %d", client.config.MaxResponsePayloadBytes, protocol.MaxPayloadCap)
	}

	client.state = StateReady
	if err := client.callWithRetry(func() error { return nil }); err != nil {
		t.Fatalf("successful retry wrapper error = %v", err)
	}
	if client.callCount != 1 {
		t.Fatalf("call count = %d, want 1", client.callCount)
	}

	client.Abort()
	if err := client.callWithRetry(func() error { return nil }); !errors.Is(err, protocol.ErrAborted) {
		t.Fatalf("aborted retry wrapper error = %v, want ErrAborted", err)
	}

	client = NewIncrementClient("", "", testLookupClientConfig())
	client.state = StateReady
	if err := client.callWithRetry(func() error { return protocol.ErrTimeout }); !errors.Is(err, protocol.ErrTimeout) {
		t.Fatalf("timeout retry wrapper error = %v, want ErrTimeout", err)
	}
	if client.state != StateBroken {
		t.Fatalf("timeout retry state = %d, want broken", client.state)
	}

	client = NewIncrementClient("", "", testLookupClientConfig())
	client.state = StateReady
	if err := client.callWithRetry(func() error { return protocol.ErrBadLayout }); !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("non-overflow retry wrapper error = %v, want ErrBadLayout", err)
	}
	if client.errorCount == 0 {
		t.Fatal("non-overflow retry did not record an error")
	}

	client = NewIncrementClient("", "", testLookupClientConfig())
	client.state = StateReady
	if err := client.callWithRetry(func() error { return protocol.ErrOverflow }); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("overflow retry wrapper error = %v, want ErrOverflow", err)
	}
	if client.errorCount == 0 {
		t.Fatal("overflow retry did not record an error")
	}
}

func TestLookupClientCapacityAndWaitHelpers(t *testing.T) {
	client := &Client{}
	client.noteRequestCapacity(33)
	if client.config.MaxRequestPayloadBytes != 64 {
		t.Fatalf("request capacity = %d, want 64", client.config.MaxRequestPayloadBytes)
	}
	client.noteRequestCapacity(32)
	if client.config.MaxRequestPayloadBytes != 64 {
		t.Fatalf("request capacity shrank to %d", client.config.MaxRequestPayloadBytes)
	}
	client.noteRequestCapacity(protocol.MaxPayloadCap + 1)
	if client.config.MaxRequestPayloadBytes != protocol.MaxPayloadCap {
		t.Fatalf("request capacity cap = %d, want %d", client.config.MaxRequestPayloadBytes, protocol.MaxPayloadCap)
	}

	if got := boundedClientWaitMs(0, 100); got != 0 {
		t.Fatalf("bounded wait zero = %d, want 0", got)
	}
	if got := boundedClientWaitMs(200*time.Millisecond, 100); got != 100 {
		t.Fatalf("bounded wait cap = %d, want 100", got)
	}
	if got := boundedClientWaitMs(1500*time.Microsecond, 100); got != 2 {
		t.Fatalf("bounded wait rounded = %d, want 2", got)
	}
}

func TestLookupReadyAndCapacityGuards(t *testing.T) {
	client := &Client{abortCh: make(chan struct{})}
	if err := client.ensureReadyForLogicalLookup(); !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("not-ready logical lookup error = %v", err)
	}
	if client.errorCount != 1 {
		t.Fatalf("not-ready error count = %d, want 1", client.errorCount)
	}

	client.state = StateReady
	client.Abort()
	if err := client.ensureReadyForLogicalLookup(); !errors.Is(err, protocol.ErrAborted) {
		t.Fatalf("aborted logical lookup error = %v", err)
	}
	if client.state != StateBroken {
		t.Fatalf("aborted logical lookup state = %d, want broken", client.state)
	}

	client = &Client{abortCh: make(chan struct{})}
	if err := client.ensureLookupRequestCapacity(-1); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("negative request capacity error = %v", err)
	}
	if strconv.IntSize >= 64 {
		if err := client.ensureLookupRequestCapacity(int(uint64(^uint32(0)) + 1)); !errors.Is(err, protocol.ErrOverflow) {
			t.Fatalf("unrepresentable request capacity error = %v", err)
		}
	}
	client = &Client{abortCh: make(chan struct{})}
	if err := client.ensureLookupRequestCapacity(int(protocol.MaxPayloadDefault) + 1); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("default request capacity error = %v", err)
	}
	client.config.MaxRequestPayloadBytes = 8
	if err := client.ensureLookupRequestCapacity(9); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("configured request capacity error = %v", err)
	}
	client.config.MaxRequestPayloadBytes = 16
	if err := client.ensureLookupRequestCapacity(16); err != nil {
		t.Fatalf("sessionless request capacity = %v", err)
	}

	client = &Client{abortCh: make(chan struct{}), state: StateReady}
	client.Abort()
	if err := client.ensureLookupRequestCapacity(1); !errors.Is(err, protocol.ErrAborted) {
		t.Fatalf("aborted capacity growth error = %v, want ErrAborted", err)
	}
	if client.state != StateBroken || client.errorCount != 1 {
		t.Fatalf("aborted capacity growth state/error = %d/%d, want broken/1", client.state, client.errorCount)
	}
}

func TestLookupWrongMethodFailsBeforeTransport(t *testing.T) {
	incrementClient := NewIncrementClient("", "", testLookupClientConfig())
	if _, err := incrementClient.CallAppsLookup(nil); !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("apps lookup on increment client error = %v, want ErrBadLayout", err)
	}
	if _, err := incrementClient.CallCgroupsLookup(nil); !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("cgroups lookup on increment client error = %v, want ErrBadLayout", err)
	}

	appsClient := NewAppsLookupClient("", "", testLookupClientConfig())
	if _, err := appsClient.CallCgroupsLookup(nil); !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("cgroups lookup on apps client error = %v, want ErrBadLayout", err)
	}

	cgroupsClient := NewCgroupsLookupClient("", "", testLookupClientConfig())
	if _, err := cgroupsClient.CallAppsLookup(nil); !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("apps lookup on cgroups client error = %v, want ErrBadLayout", err)
	}

	if _, err := appsClient.CallIncrement(1); !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("increment on apps client error = %v, want ErrBadLayout", err)
	}
	if _, err := appsClient.CallIncrementBatch([]uint64{1}); !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("increment batch on apps client error = %v, want ErrBadLayout", err)
	}
	if _, err := appsClient.CallStringReverse("abc"); !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("string reverse on apps client error = %v, want ErrBadLayout", err)
	}
	if _, err := appsClient.CallSnapshot(); !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("snapshot on apps client error = %v, want ErrBadLayout", err)
	}
}

func TestIncrementBatchRejectsUnrepresentableItemCount(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("synthetic oversized slice guard requires 64-bit int")
	}
	if _, err := incrementBatchRequestSize(int(uint64(^uint32(0)) + 1)); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("huge increment batch error = %v, want ErrOverflow", err)
	}
}

func TestLookupRequestSizeBoundaries(t *testing.T) {
	appsCount, appsSize, err := appsLookupNextRequest([]uint32{1, 2, 3, 4}, 0)
	if err != nil {
		t.Fatalf("apps default next request: %v", err)
	}
	if appsCount != 4 || appsSize == 0 {
		t.Fatalf("apps default next request = count %d size %d", appsCount, appsSize)
	}
	appsCount, appsSize, err = appsLookupNextRequest([]uint32{1, 2, 3, 4}, uint32(protocol.AppsLookupReqHdr+3*(protocol.LookupDirEntrySize+protocol.AppsLookupKeySize)))
	if err != nil {
		t.Fatalf("apps split next request: %v", err)
	}
	if appsCount != 3 || appsSize == 0 {
		t.Fatalf("apps split next request = count %d size %d", appsCount, appsSize)
	}
	if _, _, err := appsLookupNextRequest([]uint32{1}, uint32(protocol.AppsLookupReqHdr+protocol.LookupDirEntrySize+protocol.AppsLookupKeySize-1)); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("apps too-small request error = %v", err)
	}
	if count, size, err := appsLookupNextRequest(nil, 1); err != nil || count != 0 || size != protocol.AppsLookupReqHdr {
		t.Fatalf("apps empty request = count %d size %d err %v", count, size, err)
	}

	paths := [][]byte{[]byte("/a"), []byte("/bbbbbbbb"), []byte("/c")}
	cgCount, cgSize, err := cgroupsLookupNextRequest(paths, 0)
	if err != nil {
		t.Fatalf("cgroups default next request: %v", err)
	}
	if cgCount != len(paths) || cgSize == 0 {
		t.Fatalf("cgroups default next request = count %d size %d", cgCount, cgSize)
	}
	oneSize, err := cgroupsLookupRequestSize(paths[:1])
	if err != nil {
		t.Fatalf("cgroups one request size: %v", err)
	}
	cgCount, cgSize, err = cgroupsLookupNextRequest(paths, uint32(oneSize))
	if err != nil {
		t.Fatalf("cgroups split next request: %v", err)
	}
	if cgCount != 1 || cgSize != oneSize {
		t.Fatalf("cgroups split next request = count %d size %d want 1/%d", cgCount, cgSize, oneSize)
	}
	if _, _, err := cgroupsLookupNextRequest(paths, uint32(oneSize-1)); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("cgroups too-small request error = %v", err)
	}
	if count, size, err := cgroupsLookupNextRequest(nil, 1); err != nil || count != 0 || size != protocol.CgroupsLookupReqHdr {
		t.Fatalf("cgroups empty request = count %d size %d err %v", count, size, err)
	}
}

func TestCgroupsLookupOversizedRequestItem(t *testing.T) {
	rawItem, err := cgroupsLookupOversizedRequestItem([]byte("/too-big"))
	if err != nil {
		t.Fatalf("oversized request item: %v", err)
	}
	var resp [256]byte
	n, err := protocol.EncodeCgroupsLookupRawResponse([][]byte{rawItem}, 77, resp[:])
	if err != nil {
		t.Fatalf("encode oversized raw response: %v", err)
	}
	view, err := protocol.DecodeCgroupsLookupResponse(resp[:n])
	if err != nil {
		t.Fatalf("decode oversized raw response: %v", err)
	}
	item, err := view.Item(0)
	if err != nil {
		t.Fatalf("oversized item view: %v", err)
	}
	if item.Status != protocol.CgroupLookupOversizedItem || item.Path.String() != "/too-big" {
		t.Fatalf("oversized item = %+v", item)
	}

	if _, err := cgroupsLookupOversizedRequestItem([]byte("bad\x00path")); !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("invalid oversized request item error = %v", err)
	}
}

func TestLookupDispatchGuardCoverage(t *testing.T) {
	if AppsLookupDispatch(nil) != nil {
		t.Fatal("nil apps lookup handler should produce nil dispatch handler")
	}
	if CgroupsLookupDispatch(nil) != nil {
		t.Fatal("nil cgroups lookup handler should produce nil dispatch handler")
	}

	var appsReq [128]byte
	appsReqLen, err := protocol.EncodeAppsLookupRequest([]uint32{1234}, appsReq[:])
	if err != nil {
		t.Fatalf("encode apps request: %v", err)
	}
	var cgroupsReq [128]byte
	cgroupsReqLen, err := protocol.EncodeCgroupsLookupRequest([][]byte{[]byte("/known")}, cgroupsReq[:])
	if err != nil {
		t.Fatalf("encode cgroups request: %v", err)
	}

	t.Run("apps guards", func(t *testing.T) {
		success := AppsLookupDispatch(func(req *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
			pid, err := req.Item(0)
			if err != nil {
				return false
			}
			return builder.Add(protocol.PidLookupUnknown, 0, 0, pid, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil) == nil
		})
		var resp [256]byte
		n, err := success(appsReq[:appsReqLen], resp[:])
		if err != nil || n == 0 {
			t.Fatalf("apps dispatch success = n %d err %v", n, err)
		}
		if _, err := success([]byte{1, 2, 3}, resp[:]); !errors.Is(err, protocol.ErrTruncated) {
			t.Fatalf("apps bad request error = %v, want ErrTruncated", err)
		}
		if n, err := success(appsReq[:appsReqLen], make([]byte, protocol.AppsLookupRespHdr+protocol.LookupDirEntrySize-1)); n != 0 || !errors.Is(err, protocol.ErrOverflow) {
			t.Fatalf("apps short response = n %d err %v, want 0 ErrOverflow", n, err)
		}

		failing := AppsLookupDispatch(func(*protocol.AppsLookupRequestView, *protocol.AppsLookupBuilder) bool {
			return false
		})
		if n, err := failing(appsReq[:appsReqLen], resp[:]); n != 0 || !errors.Is(err, errHandlerFailed) {
			t.Fatalf("apps failing handler = n %d err %v, want 0 errHandlerFailed", n, err)
		}

		builderError := AppsLookupDispatch(func(_ *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
			_ = builder.Add(protocol.PidLookupKnown, protocol.AppsCgroupKnown, 0, 1234, 0, 0, 0, []byte("ok"), nil, nil, nil)
			return true
		})
		if n, err := builderError(appsReq[:appsReqLen], resp[:]); n != 0 || !errors.Is(err, protocol.ErrBadLayout) {
			t.Fatalf("apps builder error = n %d err %v, want 0 ErrBadLayout", n, err)
		}

		badCount := AppsLookupDispatch(func(*protocol.AppsLookupRequestView, *protocol.AppsLookupBuilder) bool {
			return true
		})
		if n, err := badCount(appsReq[:appsReqLen], resp[:]); n != 0 || !errors.Is(err, protocol.ErrBadItemCount) {
			t.Fatalf("apps bad item count = n %d err %v, want 0 ErrBadItemCount", n, err)
		}
	})

	t.Run("cgroups guards", func(t *testing.T) {
		success := CgroupsLookupDispatch(func(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
			path, err := req.Item(0)
			if err != nil {
				return false
			}
			return builder.Add(protocol.CgroupLookupUnknownRetryLater, 0, path.Bytes(), nil, nil) == nil
		})
		var resp [256]byte
		n, err := success(cgroupsReq[:cgroupsReqLen], resp[:])
		if err != nil || n == 0 {
			t.Fatalf("cgroups dispatch success = n %d err %v", n, err)
		}
		if _, err := success([]byte{1, 2, 3}, resp[:]); !errors.Is(err, protocol.ErrTruncated) {
			t.Fatalf("cgroups bad request error = %v, want ErrTruncated", err)
		}
		if n, err := success(cgroupsReq[:cgroupsReqLen], make([]byte, protocol.CgroupsLookupRespHdr+protocol.LookupDirEntrySize-1)); n != 0 || !errors.Is(err, protocol.ErrOverflow) {
			t.Fatalf("cgroups short response = n %d err %v, want 0 ErrOverflow", n, err)
		}

		failing := CgroupsLookupDispatch(func(*protocol.CgroupsLookupRequestView, *protocol.CgroupsLookupBuilder) bool {
			return false
		})
		if n, err := failing(cgroupsReq[:cgroupsReqLen], resp[:]); n != 0 || !errors.Is(err, errHandlerFailed) {
			t.Fatalf("cgroups failing handler = n %d err %v, want 0 errHandlerFailed", n, err)
		}

		builderError := CgroupsLookupDispatch(func(_ *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
			_ = builder.Add(protocol.CgroupLookupKnown, 0, nil, nil, nil)
			return true
		})
		if n, err := builderError(cgroupsReq[:cgroupsReqLen], resp[:]); n != 0 || !errors.Is(err, protocol.ErrBadLayout) {
			t.Fatalf("cgroups builder error = n %d err %v, want 0 ErrBadLayout", n, err)
		}

		badCount := CgroupsLookupDispatch(func(*protocol.CgroupsLookupRequestView, *protocol.CgroupsLookupBuilder) bool {
			return true
		})
		if n, err := badCount(cgroupsReq[:cgroupsReqLen], resp[:]); n != 0 || !errors.Is(err, protocol.ErrBadItemCount) {
			t.Fatalf("cgroups bad item count = n %d err %v, want 0 ErrBadItemCount", n, err)
		}
	})
}

func TestSimpleRawDispatchGuardCoverage(t *testing.T) {
	if IncrementDispatch(nil) != nil {
		t.Fatal("nil increment handler should produce nil dispatch handler")
	}
	if StringReverseDispatch(nil) != nil {
		t.Fatal("nil string reverse handler should produce nil dispatch handler")
	}

	t.Run("increment", func(t *testing.T) {
		var req [protocol.IncrementPayloadSize]byte
		if n := protocol.IncrementEncode(41, req[:]); n == 0 {
			t.Fatal("encode increment request")
		}
		dispatch := IncrementDispatch(func(v uint64) (uint64, bool) {
			return v + 1, true
		})
		var resp [protocol.IncrementPayloadSize]byte
		n, err := dispatch(req[:], resp[:])
		if err != nil || n != protocol.IncrementPayloadSize {
			t.Fatalf("increment dispatch success = n %d err %v", n, err)
		}
		value, err := protocol.IncrementDecode(resp[:n])
		if err != nil || value != 42 {
			t.Fatalf("increment dispatch value = %d err %v", value, err)
		}
		if n, err := dispatch(req[:1], resp[:]); n != 0 || !errors.Is(err, protocol.ErrTruncated) {
			t.Fatalf("increment bad request = n %d err %v, want 0 ErrTruncated", n, err)
		}
		failing := IncrementDispatch(func(uint64) (uint64, bool) {
			return 0, false
		})
		if n, err := failing(req[:], resp[:]); n != 0 || !errors.Is(err, errHandlerFailed) {
			t.Fatalf("increment failing handler = n %d err %v, want 0 errHandlerFailed", n, err)
		}
		if n, err := dispatch(req[:], resp[:1]); n != 0 || !errors.Is(err, protocol.ErrOverflow) {
			t.Fatalf("increment short response = n %d err %v, want 0 ErrOverflow", n, err)
		}
	})

	t.Run("string reverse", func(t *testing.T) {
		req := make([]byte, protocol.StringReverseHdrSize+4)
		if n := protocol.StringReverseEncode("abc", req); n == 0 {
			t.Fatal("encode string reverse request")
		}
		dispatch := StringReverseDispatch(func(s string) (string, bool) {
			return "cba", true
		})
		resp := make([]byte, protocol.StringReverseHdrSize+4)
		n, err := dispatch(req, resp)
		if err != nil || n == 0 {
			t.Fatalf("string reverse dispatch success = n %d err %v", n, err)
		}
		view, err := protocol.StringReverseDecode(resp[:n])
		if err != nil || view.Str != "cba" {
			t.Fatalf("string reverse dispatch view = %+v err %v", view, err)
		}
		if n, err := dispatch(req[:1], resp); n != 0 || !errors.Is(err, protocol.ErrTruncated) {
			t.Fatalf("string reverse bad request = n %d err %v, want 0 ErrTruncated", n, err)
		}
		failing := StringReverseDispatch(func(string) (string, bool) {
			return "", false
		})
		if n, err := failing(req, resp); n != 0 || !errors.Is(err, errHandlerFailed) {
			t.Fatalf("string reverse failing handler = n %d err %v, want 0 errHandlerFailed", n, err)
		}
		if n, err := dispatch(req, resp[:1]); n != 0 || !errors.Is(err, protocol.ErrOverflow) {
			t.Fatalf("string reverse short response = n %d err %v, want 0 ErrOverflow", n, err)
		}
	})
}

func TestSnapshotDispatchGuardCoverage(t *testing.T) {
	if SnapshotDispatch(nil, 1) != nil {
		t.Fatal("nil snapshot handler should produce nil dispatch handler")
	}

	var req [4]byte
	snapReq := protocol.CgroupsRequest{LayoutVersion: 1}
	if snapReq.Encode(req[:]) == 0 {
		t.Fatal("encode snapshot request")
	}

	dispatch := SnapshotDispatch(func(_ *protocol.CgroupsRequest, builder *protocol.CgroupsBuilder) bool {
		return builder.Add(1, 0, 1, []byte("name"), []byte("/path")) == nil
	}, 1)
	var resp [128]byte
	n, err := dispatch(req[:], resp[:])
	if err != nil || n == 0 {
		t.Fatalf("snapshot dispatch success = n %d err %v", n, err)
	}
	if _, err := protocol.DecodeCgroupsResponse(resp[:n]); err != nil {
		t.Fatalf("snapshot decode: %v", err)
	}
	if n, err := dispatch(req[:1], resp[:]); n != 0 || !errors.Is(err, protocol.ErrTruncated) {
		t.Fatalf("snapshot bad request = n %d err %v, want 0 ErrTruncated", n, err)
	}
	minSnapshotBytes, ok := protocol.CgroupsBuilderMinBytes(1)
	if !ok {
		t.Fatal("snapshot min bytes overflow")
	}
	if n, err := dispatch(req[:], resp[:minSnapshotBytes-1]); n != 0 || !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("snapshot short response = n %d err %v, want 0 ErrOverflow", n, err)
	}

	failing := SnapshotDispatch(func(*protocol.CgroupsRequest, *protocol.CgroupsBuilder) bool {
		return false
	}, 1)
	if n, err := failing(req[:], resp[:]); n != 0 || !errors.Is(err, errHandlerFailed) {
		t.Fatalf("snapshot failing handler = n %d err %v, want 0 errHandlerFailed", n, err)
	}

	zeroBudget := SnapshotDispatch(func(*protocol.CgroupsRequest, *protocol.CgroupsBuilder) bool {
		return true
	}, 0)
	minZeroBudgetBytes, ok := protocol.CgroupsBuilderMinBytes(0)
	if !ok {
		t.Fatal("snapshot zero-budget min bytes overflow")
	}
	if n, err := zeroBudget(req[:], make([]byte, minZeroBudgetBytes)); n != 0 || !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("snapshot zero budget = n %d err %v, want 0 ErrOverflow", n, err)
	}
}
