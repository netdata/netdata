package protocol

import (
	"bytes"
	"errors"
	"strconv"
	"testing"
)

func TestLookupRequestEncodeAndViewGuards(t *testing.T) {
	if n, err := EncodeAppsLookupRequest([]uint32{1}, make([]byte, AppsLookupReqHdr+LookupDirEntrySize+AppsLookupKeySize-1)); n != 0 || !errors.Is(err, ErrOverflow) {
		t.Fatalf("apps encode short buffer = n %d err %v, want 0 ErrOverflow", n, err)
	}
	var apps [128]byte
	appsLen, err := EncodeAppsLookupRequest([]uint32{1}, apps[:])
	if err != nil {
		t.Fatalf("apps encode request: %v", err)
	}
	appsView, err := DecodeAppsLookupRequest(apps[:appsLen])
	if err != nil {
		t.Fatalf("apps decode request: %v", err)
	}
	if _, err := appsView.Item(1); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("apps request item out of bounds = %v, want ErrOutOfBounds", err)
	}
	truncatedApps := make([]byte, AppsLookupReqHdr)
	ne.PutUint16(truncatedApps[0:2], 1)
	ne.PutUint32(truncatedApps[4:8], 1)
	if _, err := DecodeAppsLookupRequest(truncatedApps); !errors.Is(err, ErrTruncated) {
		t.Fatalf("apps truncated directory decode = %v, want ErrTruncated", err)
	}

	path := []byte("/known")
	if n, err := EncodeCgroupsLookupRequest([][]byte{path}, make([]byte, CgroupsLookupReqHdr+LookupDirEntrySize-1)); n != 0 || !errors.Is(err, ErrOverflow) {
		t.Fatalf("cgroups encode short directory buffer = n %d err %v, want 0 ErrOverflow", n, err)
	}
	oneSize := CgroupsLookupReqHdr + LookupDirEntrySize + len(path) + 1
	if n, err := EncodeCgroupsLookupRequest([][]byte{path}, make([]byte, oneSize-1)); n != 0 || !errors.Is(err, ErrOverflow) {
		t.Fatalf("cgroups encode short payload buffer = n %d err %v, want 0 ErrOverflow", n, err)
	}
	var cgroups [128]byte
	cgroupsLen, err := EncodeCgroupsLookupRequest([][]byte{path}, cgroups[:])
	if err != nil {
		t.Fatalf("cgroups encode request: %v", err)
	}
	cgroupsView, err := DecodeCgroupsLookupRequest(cgroups[:cgroupsLen])
	if err != nil {
		t.Fatalf("cgroups decode request: %v", err)
	}
	if _, err := cgroupsView.Item(1); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("cgroups request item out of bounds = %v, want ErrOutOfBounds", err)
	}
	truncatedCgroups := make([]byte, CgroupsLookupReqHdr)
	ne.PutUint16(truncatedCgroups[0:2], 1)
	ne.PutUint32(truncatedCgroups[4:8], 1)
	if _, err := DecodeCgroupsLookupRequest(truncatedCgroups); !errors.Is(err, ErrTruncated) {
		t.Fatalf("cgroups truncated directory decode = %v, want ErrTruncated", err)
	}
}

func TestLookupSyntheticArithmeticGuards(t *testing.T) {
	maxInt := maxIntValue()

	if suffixBytes, ok := buildPayloadExceededSuffixBytes([]int{4, 4}); !ok || len(suffixBytes) != 3 || suffixBytes[0] != 12 || suffixBytes[1] != 4 || suffixBytes[2] != 0 {
		t.Fatalf("suffix bytes = %v ok=%v, want [12 4 0] true", suffixBytes, ok)
	}
	if _, ok := checkedAddU32(^uint32(0), 1); ok {
		t.Fatal("checkedAddU32 should reject overflow")
	}
	if _, ok := checkedAlign8U32(^uint32(0) - 3); ok {
		t.Fatal("checkedAlign8U32 should reject overflow")
	}
	if !finishPayloadExceededSuffixBytes(nil) {
		t.Fatal("empty payload-exceeded suffix should finalize")
	}
	if payloadExceededSuffixFits(16, 0, []uint32{1}, 0, 2) != true {
		t.Fatal("mismatched suffix byte table should be treated as non-actionable")
	}
	if payloadExceededSuffixFits(16, 0, []uint32{1, 0}, 2, 1) != true {
		t.Fatal("suffix starting after max items should fit vacuously")
	}
	if payloadExceededSuffixFits(16, maxInt, []uint32{1, 0}, 0, 1) != false {
		t.Fatal("suffix with overflowing alignment should not fit")
	}
	if payloadExceededSuffixFits(16, 0, []uint32{^uint32(0), 0}, 0, 1) != false {
		t.Fatal("suffix with overflowing item end should not fit")
	}
	if payloadExceededSuffixFits(16, 0, []uint32{12, 4, 0}, 1, 2) != true {
		t.Fatal("valid suffix should fit")
	}
	if !payloadExceededFixedSuffixFits(16, 0, 4, 1, 2) {
		t.Fatal("valid fixed suffix should fit")
	}
	if payloadExceededFixedSuffixFits(11, 0, 4, 0, 2) {
		t.Fatal("oversized fixed suffix should not fit")
	}

	if _, _, _, _, _, _, ok := lookupLabelWriteLayout(-1, 1, 1); ok {
		t.Fatal("negative label key offset should fail")
	}
	if _, _, _, _, _, _, ok := lookupLabelWriteLayout(0, -1, 1); ok {
		t.Fatal("negative label key length should fail")
	}
	if _, _, _, _, _, _, ok := lookupLabelWriteLayout(maxInt, 1, 1); ok {
		t.Fatal("overflowing label value offset should fail")
	}
	if strconv.IntSize >= 64 {
		maxU32Value := ^uint32(0)
		overU32 := int(uint64(maxU32Value) + 1)
		if _, _, _, _, _, _, ok := lookupLabelWriteLayout(overU32, 1, 1); ok {
			t.Fatal("label key offset above uint32 should fail")
		}
		if _, _, _, _, _, _, ok := lookupLabelWriteLayout(0, overU32, 1); ok {
			t.Fatal("label key length above uint32 should fail")
		}
		if _, _, _, _, _, _, ok := lookupLabelWriteLayout(int(maxU32Value), 0, 1); ok {
			t.Fatal("label value offset above uint32 should fail")
		}
		if _, _, _, _, _, _, ok := lookupLabelWriteLayout(0, 0, overU32); ok {
			t.Fatal("label value length above uint32 should fail")
		}
	}

	if _, _, _, err := labelLayoutPrefix(maxInt, 1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("label layout align overflow = %v, want ErrOverflow", err)
	}
	if _, _, _, err := labelLayoutPrefix(0, maxInt/LookupLabelEntrySize+1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("label table size overflow = %v, want ErrOverflow", err)
	}
	if _, _, _, err := labelLayoutGo(16, []struct{ Key, Value []byte }{{Key: []byte{}, Value: []byte("v")}}); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("empty label key layout error = %v, want ErrBadLayout", err)
	}

	if _, ok := lookupBuilderDataOffset(maxInt, 1); ok {
		t.Fatal("lookup builder data offset should fail when header plus directory overflows")
	}
	if _, ok := lookupDirOffset(maxInt, 1); ok {
		t.Fatal("lookup directory offset should fail when header plus entry offset overflows")
	}
	if _, _, ok := lookupRawResponseMinBytes(0, -1); ok {
		t.Fatal("negative raw response item count should fail")
	}
	if _, _, ok := lookupRawResponseMinBytes(maxInt, 1); ok {
		t.Fatal("raw response min bytes should fail when header plus directory overflows")
	}
	if _, _, ok := lookupRawResponseItemBounds(maxInt, 1); ok {
		t.Fatal("raw response item bounds should fail on alignment overflow")
	}
	if _, _, ok := lookupRawResponseItemBounds(0, -1); ok {
		t.Fatal("raw response item bounds should fail on invalid item length")
	}

	if _, _, ok := appsLookupRequestLayoutForCount(-1); ok {
		t.Fatal("negative apps lookup request count should fail")
	}
	if _, _, ok := appsLookupRequestLayoutForCount(maxInt/LookupDirEntrySize + 1); ok {
		t.Fatal("apps lookup request directory size should overflow")
	}
	if _, _, ok := appsLookupRequestLayoutForCount(maxInt/AppsLookupKeySize + 1); ok {
		t.Fatal("apps lookup request key size should overflow")
	}
	if _, ok := cgroupsLookupRequestPackedStartForCount(-1); ok {
		t.Fatal("negative cgroups lookup request count should fail")
	}
	if _, ok := cgroupsLookupRequestPackedStartForCount(maxInt/LookupDirEntrySize + 1); ok {
		t.Fatal("cgroups lookup request directory size should overflow")
	}
	if _, ok := cgroupsLookupRequestPackedStartForCount(maxInt / LookupDirEntrySize); ok {
		t.Fatal("cgroups lookup request packed start should overflow")
	}
	if strconv.IntSize >= 64 {
		maxU32Value := ^uint32(0)
		overU32 := int(uint64(maxU32Value) + 1)
		if _, _, ok := appsLookupRequestLayoutForCount(overU32); ok {
			t.Fatal("apps lookup request count above uint32 should fail")
		}
		if _, ok := cgroupsLookupRequestPackedStartForCount(overU32); ok {
			t.Fatal("cgroups lookup request count above uint32 should fail")
		}
	}

	if _, _, _, ok := cgroupsItemLayoutForLengths(-1, 1); ok {
		t.Fatal("negative cgroups item name length should fail")
	}
	if _, _, _, ok := cgroupsItemLayoutForLengths(1, -1); ok {
		t.Fatal("negative cgroups item path length should fail")
	}
	if _, _, _, ok := cgroupsItemLayoutForLengths(maxInt, 1); ok {
		t.Fatal("overflowing cgroups item name size should fail")
	}
	if _, _, _, ok := cgroupsItemLayoutForLengths(1, maxInt); ok {
		t.Fatal("overflowing cgroups item path size should fail")
	}
	if strconv.IntSize >= 64 {
		maxU32Value := ^uint32(0)
		maxU32 := int(maxU32Value)
		if _, _, _, ok := cgroupsItemLayoutForLengths(maxU32, 1); ok {
			t.Fatal("cgroups item path offset above uint32 should fail")
		}
		if _, _, _, ok := cgroupsItemLayoutForLengths(0, maxU32); ok {
			t.Fatal("cgroups item size above uint32 should fail")
		}
	}
}

func TestLookupRequestDecodersRejectMalformedKeys(t *testing.T) {
	var apps [128]byte
	appsLen, err := EncodeAppsLookupRequest([]uint32{1234}, apps[:])
	if err != nil {
		t.Fatalf("apps encode request: %v", err)
	}
	appsBadReserved := append([]byte(nil), apps[:appsLen]...)
	dirEnd := AppsLookupReqHdr + LookupDirEntrySize
	ne.PutUint32(appsBadReserved[dirEnd+4:dirEnd+8], 1)
	if _, err := DecodeAppsLookupRequest(appsBadReserved); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("apps reserved key error = %v, want ErrBadLayout", err)
	}

	var cgroups [128]byte
	cgroupsLen, err := EncodeCgroupsLookupRequest([][]byte{[]byte("/abc")}, cgroups[:])
	if err != nil {
		t.Fatalf("cgroups encode request: %v", err)
	}
	cgroupsDirEnd := CgroupsLookupReqHdr + LookupDirEntrySize
	cgroupsMissingNul := append([]byte(nil), cgroups[:cgroupsLen]...)
	cgroupsMissingNul[cgroupsLen-1] = 'x'
	if _, err := DecodeCgroupsLookupRequest(cgroupsMissingNul); !errors.Is(err, ErrMissingNul) {
		t.Fatalf("cgroups missing nul error = %v, want ErrMissingNul", err)
	}
	cgroupsInteriorNul := append([]byte(nil), cgroups[:cgroupsLen]...)
	cgroupsInteriorNul[cgroupsDirEnd+1] = 0
	if _, err := DecodeCgroupsLookupRequest(cgroupsInteriorNul); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("cgroups interior nul error = %v, want ErrBadLayout", err)
	}
}

func TestLookupResponseViewGuardCoverage(t *testing.T) {
	var apps [256]byte
	appsBuilder := NewAppsLookupBuilder(apps[:], 1, 10)
	if err := appsBuilder.Add(PidLookupUnknown, 0, 0, 1234, 0, NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
		t.Fatalf("apps add unknown: %v", err)
	}
	appsLen := appsBuilder.Finish()
	appsView, err := DecodeAppsLookupResponse(apps[:appsLen])
	if err != nil {
		t.Fatalf("apps decode response: %v", err)
	}
	if _, err := appsView.Item(1); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("apps response item out of bounds = %v, want ErrOutOfBounds", err)
	}
	if _, err := appsView.RawItem(1); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("apps raw item out of bounds = %v, want ErrOutOfBounds", err)
	}
	truncatedApps := make([]byte, AppsLookupRespHdr)
	ne.PutUint16(truncatedApps[0:2], 1)
	ne.PutUint32(truncatedApps[4:8], 1)
	if _, err := DecodeAppsLookupResponse(truncatedApps); !errors.Is(err, ErrTruncated) {
		t.Fatalf("apps truncated response directory = %v, want ErrTruncated", err)
	}

	var cgroups [256]byte
	cgroupsBuilder := NewCgroupsLookupBuilder(cgroups[:], 1, 11)
	if err := cgroupsBuilder.Add(CgroupLookupUnknownRetryLater, 0, []byte("/known"), nil, nil); err != nil {
		t.Fatalf("cgroups add unknown: %v", err)
	}
	cgroupsLen := cgroupsBuilder.Finish()
	cgroupsView, err := DecodeCgroupsLookupResponse(cgroups[:cgroupsLen])
	if err != nil {
		t.Fatalf("cgroups decode response: %v", err)
	}
	if _, err := cgroupsView.Item(1); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("cgroups response item out of bounds = %v, want ErrOutOfBounds", err)
	}
	if _, err := cgroupsView.RawItem(1); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("cgroups raw item out of bounds = %v, want ErrOutOfBounds", err)
	}
	truncatedCgroups := make([]byte, CgroupsLookupRespHdr)
	ne.PutUint16(truncatedCgroups[0:2], 1)
	ne.PutUint32(truncatedCgroups[4:8], 1)
	if _, err := DecodeCgroupsLookupResponse(truncatedCgroups); !errors.Is(err, ErrTruncated) {
		t.Fatalf("cgroups truncated response directory = %v, want ErrTruncated", err)
	}
}

func TestLookupResponseLabelsRoundTrip(t *testing.T) {
	labels := []struct{ Key, Value []byte }{
		{Key: []byte("container"), Value: []byte("api")},
		{Key: []byte("zone"), Value: []byte("prod")},
	}

	var apps [512]byte
	appsBuilder := NewAppsLookupBuilder(apps[:], 1, 100)
	if err := appsBuilder.Add(
		PidLookupKnown,
		AppsCgroupKnown,
		OrchestratorDocker,
		1234,
		1,
		1000,
		42,
		[]byte("worker"),
		[]byte("/docker/abc"),
		[]byte("api"),
		labels,
	); err != nil {
		t.Fatalf("add apps labels: %v", err)
	}
	appsLen := appsBuilder.Finish()
	appsView, err := DecodeAppsLookupResponse(apps[:appsLen])
	if err != nil {
		t.Fatalf("decode apps labels: %v", err)
	}
	appsItem, err := appsView.Item(0)
	if err != nil {
		t.Fatalf("apps item: %v", err)
	}
	if appsItem.LabelCount != uint16(len(labels)) {
		t.Fatalf("apps label count = %d, want %d", appsItem.LabelCount, len(labels))
	}
	for i, want := range labels {
		label, err := appsItem.Label(uint32(i))
		if err != nil {
			t.Fatalf("apps label %d: %v", i, err)
		}
		if label.Key.String() != string(want.Key) || label.Value.String() != string(want.Value) {
			t.Fatalf("apps label %d = %q/%q, want %q/%q", i, label.Key.String(), label.Value.String(), want.Key, want.Value)
		}
	}
	if _, err := appsItem.Label(uint32(len(labels))); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("apps label out of bounds = %v, want ErrOutOfBounds", err)
	}

	var cgroups [512]byte
	cgroupsBuilder := NewCgroupsLookupBuilder(cgroups[:], 1, 101)
	if err := cgroupsBuilder.Add(CgroupLookupKnown, OrchestratorK8s, []byte("/kubepods/pod"), []byte("pod"), labels); err != nil {
		t.Fatalf("add cgroups labels: %v", err)
	}
	cgroupsLen := cgroupsBuilder.Finish()
	cgroupsView, err := DecodeCgroupsLookupResponse(cgroups[:cgroupsLen])
	if err != nil {
		t.Fatalf("decode cgroups labels: %v", err)
	}
	cgroupsItem, err := cgroupsView.Item(0)
	if err != nil {
		t.Fatalf("cgroups item: %v", err)
	}
	if cgroupsItem.LabelCount != uint16(len(labels)) {
		t.Fatalf("cgroups label count = %d, want %d", cgroupsItem.LabelCount, len(labels))
	}
	for i, want := range labels {
		label, err := cgroupsItem.Label(uint32(i))
		if err != nil {
			t.Fatalf("cgroups label %d: %v", i, err)
		}
		if label.Key.String() != string(want.Key) || label.Value.String() != string(want.Value) {
			t.Fatalf("cgroups label %d = %q/%q, want %q/%q", i, label.Key.String(), label.Value.String(), want.Key, want.Value)
		}
	}
	if _, err := cgroupsItem.Label(uint32(len(labels))); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("cgroups label out of bounds = %v, want ErrOutOfBounds", err)
	}
}

func TestLookupValidateLabelsRejectsMalformedLayouts(t *testing.T) {
	if _, err := validateLabels(make([]byte, 2), 0, 0, 1); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("zero labels with trailing data = %v, want ErrBadLayout", err)
	}
	if _, err := validateLabels(nil, 0, 1, maxIntValue()); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("label table alignment overflow = %v, want ErrOutOfBounds", err)
	}
	if _, err := validateLabels(make([]byte, 4), 0, 1, 8); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("label table starts past item = %v, want ErrOutOfBounds", err)
	}

	padded := make([]byte, 8)
	padded[1] = 1
	if _, err := validateLabels(padded, 0, 1, 1); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("nonzero label table padding = %v, want ErrBadLayout", err)
	}
	if _, err := validateLabels(make([]byte, 8), 0, 1, 8); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("truncated label table = %v, want ErrOutOfBounds", err)
	}

	keyOffsetMismatch := make([]byte, 24)
	ne.PutUint32(keyOffsetMismatch[8:12], 25)
	ne.PutUint32(keyOffsetMismatch[12:16], 1)
	if _, err := validateLabels(keyOffsetMismatch, 0, 1, 8); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("label key offset mismatch = %v, want ErrBadLayout", err)
	}

	valueOffsetMismatch := make([]byte, 28)
	ne.PutUint32(valueOffsetMismatch[8:12], 24)
	ne.PutUint32(valueOffsetMismatch[12:16], 1)
	ne.PutUint32(valueOffsetMismatch[16:20], 27)
	valueOffsetMismatch[25] = 0
	if _, err := validateLabels(valueOffsetMismatch, 0, 1, 8); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("label value offset mismatch = %v, want ErrBadLayout", err)
	}

	trailing := make([]byte, 28)
	ne.PutUint32(trailing[8:12], 24)
	ne.PutUint32(trailing[12:16], 1)
	ne.PutUint32(trailing[16:20], 26)
	trailing[25] = 0
	trailing[26] = 0
	if _, err := validateLabels(trailing, 0, 1, 8); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("label trailing data = %v, want ErrBadLayout", err)
	}
}

func buildAppsLookupBoundaryResponse(t *testing.T, buf []byte) int {
	t.Helper()

	builder := NewAppsLookupBuilder(buf, 2, 222)
	if err := builder.Add(
		PidLookupKnown,
		AppsCgroupHostRoot,
		0,
		1234,
		1,
		1000,
		42,
		[]byte("a"),
		nil,
		nil,
		nil,
	); err != nil {
		t.Fatalf("apps boundary add item 0: %v", err)
	}
	if err := builder.Add(
		PidLookupKnown,
		AppsCgroupKnown,
		OrchestratorDocker,
		5678,
		1,
		1000,
		43,
		[]byte("worker"),
		[]byte("/docker/long-container-path"),
		[]byte("container-name"),
		labels(struct{ Key, Value []byte }{[]byte("role"), []byte("api")}),
	); err != nil {
		t.Fatalf("apps boundary add item 1: %v", err)
	}
	return builder.Finish()
}

func buildCgroupsLookupBoundaryResponse(t *testing.T, buf []byte) int {
	t.Helper()

	builder := NewCgroupsLookupBuilder(buf, 2, 333)
	if err := builder.Add(
		CgroupLookupKnown,
		OrchestratorK8s,
		[]byte("/kubepods/pod-a"),
		[]byte("pod-a"),
		nil,
	); err != nil {
		t.Fatalf("cgroups boundary add item 0: %v", err)
	}
	if err := builder.Add(
		CgroupLookupKnown,
		OrchestratorK8s,
		[]byte("/kubepods/long-pod-path"),
		[]byte("long-pod-name"),
		labels(struct{ Key, Value []byte }{[]byte("namespace"), []byte("default")}),
	); err != nil {
		t.Fatalf("cgroups boundary add item 1: %v", err)
	}
	return builder.Finish()
}

func TestLookupResponseExactAndShortBoundary(t *testing.T) {
	var appsLarge [1024]byte
	appsExactLen := buildAppsLookupBoundaryResponse(t, appsLarge[:])
	if appsExactLen <= AppsLookupRespHdr+2*LookupDirEntrySize {
		t.Fatalf("apps exact response length = %d, want payload beyond header and directory", appsExactLen)
	}

	appsExact := make([]byte, appsExactLen)
	if got := buildAppsLookupBoundaryResponse(t, appsExact); got != appsExactLen {
		t.Fatalf("apps exact-fit length = %d, want %d", got, appsExactLen)
	}
	appsExactView, err := DecodeAppsLookupResponse(appsExact)
	if err != nil {
		t.Fatalf("decode apps exact response: %v", err)
	}
	appsExactItem, err := appsExactView.Item(1)
	if err != nil {
		t.Fatalf("apps exact item 1: %v", err)
	}
	if appsExactItem.Status != PidLookupKnown || appsExactItem.Pid != 5678 {
		t.Fatalf("apps exact item 1 = status %d pid %d, want known pid 5678", appsExactItem.Status, appsExactItem.Pid)
	}

	appsPlusOne := make([]byte, appsExactLen+1)
	if got := buildAppsLookupBoundaryResponse(t, appsPlusOne); got != appsExactLen {
		t.Fatalf("apps plus-one length = %d, want %d", got, appsExactLen)
	}
	appsPlusOneView, err := DecodeAppsLookupResponse(appsPlusOne[:appsExactLen])
	if err != nil {
		t.Fatalf("decode apps plus-one response: %v", err)
	}
	appsPlusOneItem, err := appsPlusOneView.Item(1)
	if err != nil {
		t.Fatalf("apps plus-one item 1: %v", err)
	}
	if appsPlusOneItem.Status != PidLookupKnown || appsPlusOneItem.Pid != 5678 {
		t.Fatalf("apps plus-one item 1 = status %d pid %d, want known pid 5678", appsPlusOneItem.Status, appsPlusOneItem.Pid)
	}

	appsShort := make([]byte, appsExactLen-1)
	appsShortLen := buildAppsLookupBoundaryResponse(t, appsShort)
	appsShortView, err := DecodeAppsLookupResponse(appsShort[:appsShortLen])
	if err != nil {
		t.Fatalf("decode apps short response: %v", err)
	}
	appsShortItem, err := appsShortView.Item(1)
	if err != nil {
		t.Fatalf("apps short item 1: %v", err)
	}
	if appsShortItem.Status != PidLookupPayloadExceeded || appsShortItem.Pid != 5678 {
		t.Fatalf("apps short item 1 = status %d pid %d, want payload-exceeded pid 5678", appsShortItem.Status, appsShortItem.Pid)
	}

	var cgroupsLarge [1024]byte
	cgroupsExactLen := buildCgroupsLookupBoundaryResponse(t, cgroupsLarge[:])
	if cgroupsExactLen <= CgroupsLookupRespHdr+2*LookupDirEntrySize {
		t.Fatalf("cgroups exact response length = %d, want payload beyond header and directory", cgroupsExactLen)
	}

	cgroupsExact := make([]byte, cgroupsExactLen)
	if got := buildCgroupsLookupBoundaryResponse(t, cgroupsExact); got != cgroupsExactLen {
		t.Fatalf("cgroups exact-fit length = %d, want %d", got, cgroupsExactLen)
	}
	cgroupsExactView, err := DecodeCgroupsLookupResponse(cgroupsExact)
	if err != nil {
		t.Fatalf("decode cgroups exact response: %v", err)
	}
	cgroupsExactItem, err := cgroupsExactView.Item(1)
	if err != nil {
		t.Fatalf("cgroups exact item 1: %v", err)
	}
	if cgroupsExactItem.Status != CgroupLookupKnown || cgroupsExactItem.Path.String() != "/kubepods/long-pod-path" {
		t.Fatalf("cgroups exact item 1 = status %d path %q, want known long path", cgroupsExactItem.Status, cgroupsExactItem.Path.String())
	}

	cgroupsPlusOne := make([]byte, cgroupsExactLen+1)
	if got := buildCgroupsLookupBoundaryResponse(t, cgroupsPlusOne); got != cgroupsExactLen {
		t.Fatalf("cgroups plus-one length = %d, want %d", got, cgroupsExactLen)
	}
	cgroupsPlusOneView, err := DecodeCgroupsLookupResponse(cgroupsPlusOne[:cgroupsExactLen])
	if err != nil {
		t.Fatalf("decode cgroups plus-one response: %v", err)
	}
	cgroupsPlusOneItem, err := cgroupsPlusOneView.Item(1)
	if err != nil {
		t.Fatalf("cgroups plus-one item 1: %v", err)
	}
	if cgroupsPlusOneItem.Status != CgroupLookupKnown || cgroupsPlusOneItem.Path.String() != "/kubepods/long-pod-path" {
		t.Fatalf("cgroups plus-one item 1 = status %d path %q, want known long path", cgroupsPlusOneItem.Status, cgroupsPlusOneItem.Path.String())
	}

	cgroupsShort := make([]byte, cgroupsExactLen-1)
	cgroupsShortLen := buildCgroupsLookupBoundaryResponse(t, cgroupsShort)
	cgroupsShortView, err := DecodeCgroupsLookupResponse(cgroupsShort[:cgroupsShortLen])
	if err != nil {
		t.Fatalf("decode cgroups short response: %v", err)
	}
	cgroupsShortItem, err := cgroupsShortView.Item(1)
	if err != nil {
		t.Fatalf("cgroups short item 1: %v", err)
	}
	if cgroupsShortItem.Status != CgroupLookupPayloadExceeded || cgroupsShortItem.Path.String() != "/kubepods/long-pod-path" {
		t.Fatalf("cgroups short item 1 = status %d path %q, want payload-exceeded long path", cgroupsShortItem.Status, cgroupsShortItem.Path.String())
	}
}

func TestCgroupsSnapshotGuardCoverage(t *testing.T) {
	if got := EstimateCgroupsMaxItems(cgroupsRespHdr); got != 0 {
		t.Fatalf("estimate at header size = %d, want 0", got)
	}

	var buf [256]byte
	builder := NewCgroupsBuilder(buf[:], 1, 1, 99)
	if err := builder.Add(1, 0, 1, []byte("name"), []byte("/path")); err != nil {
		t.Fatalf("add cgroup item: %v", err)
	}
	total := builder.Finish()
	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatalf("decode cgroups response: %v", err)
	}
	if _, err := view.Item(1); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("snapshot item out of bounds = %v, want ErrOutOfBounds", err)
	}

	badFlags := append([]byte(nil), buf[:total]...)
	dirStart := cgroupsRespHdr
	packedStart := cgroupsRespHdr + cgroupsDirEntry
	itemStart := packedStart + int(ne.Uint32(badFlags[dirStart:dirStart+4]))
	ne.PutUint16(badFlags[itemStart+2:itemStart+4], 1)
	badView, err := DecodeCgroupsResponse(badFlags)
	if err != nil {
		t.Fatalf("decode bad-flags envelope: %v", err)
	}
	if _, err := badView.Item(0); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("snapshot item flags error = %v, want ErrBadLayout", err)
	}

	item := buf[itemStart:total]
	nameOff := int(ne.Uint32(item[16:20]))
	nameLen := int(ne.Uint32(item[20:24]))
	pathOff := int(ne.Uint32(item[24:28]))
	pathLen := int(ne.Uint32(item[28:32]))

	for _, tc := range []struct {
		name string
		edit func([]byte)
		want error
	}{
		{
			name: "snapshot item bad layout version",
			edit: func(bad []byte) {
				ne.PutUint16(bad[itemStart:itemStart+2], 2)
			},
			want: ErrBadLayout,
		},
		{
			name: "snapshot name offset before header",
			edit: func(bad []byte) {
				ne.PutUint32(bad[itemStart+16:itemStart+20], cgroupsItemHdr-1)
			},
			want: ErrOutOfBounds,
		},
		{
			name: "snapshot name missing nul",
			edit: func(bad []byte) {
				bad[itemStart+nameOff+nameLen] = 'x'
			},
			want: ErrMissingNul,
		},
		{
			name: "snapshot path offset before header",
			edit: func(bad []byte) {
				ne.PutUint32(bad[itemStart+24:itemStart+28], cgroupsItemHdr-1)
			},
			want: ErrOutOfBounds,
		},
		{
			name: "snapshot path missing nul",
			edit: func(bad []byte) {
				bad[itemStart+pathOff+pathLen] = 'x'
			},
			want: ErrMissingNul,
		},
		{
			name: "snapshot path overlaps name terminator",
			edit: func(bad []byte) {
				ne.PutUint32(bad[itemStart+24:itemStart+28], uint32(nameOff+nameLen))
				ne.PutUint32(bad[itemStart+28:itemStart+32], 0)
			},
			want: ErrBadLayout,
		},
	} {
		bad := append([]byte(nil), buf[:total]...)
		tc.edit(bad)
		view, err := DecodeCgroupsResponse(bad)
		if err != nil {
			t.Fatalf("%s envelope decode: %v", tc.name, err)
		}
		if _, err := view.Item(0); !errors.Is(err, tc.want) {
			t.Fatalf("%s error = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestLookupMalformedKnownItemsRejectFieldCorruption(t *testing.T) {
	var apps [512]byte
	appsBuilder := NewAppsLookupBuilder(apps[:], 1, 1)
	if err := appsBuilder.Add(
		PidLookupKnown,
		AppsCgroupKnown,
		OrchestratorDocker,
		1234,
		1,
		1000,
		42,
		[]byte("ok"),
		[]byte("/p"),
		[]byte("n"),
		nil,
	); err != nil {
		t.Fatalf("add apps known item: %v", err)
	}
	appsLen := appsBuilder.Finish()
	appsItemStart := appsLookupItemStart(t, apps[:appsLen])
	appsItem := apps[appsItemStart:appsLen]
	appsCommOff := int(ne.Uint32(appsItem[32:36]))
	appsPathOff := int(ne.Uint32(appsItem[40:44]))
	appsNameOff := int(ne.Uint32(appsItem[48:52]))

	for _, tc := range []struct {
		name string
		edit func([]byte)
		want error
	}{
		{
			name: "apps comm offset before header",
			edit: func(buf []byte) {
				ne.PutUint32(buf[appsItemStart+32:appsItemStart+36], AppsLookupItemHdr-1)
			},
			want: ErrOutOfBounds,
		},
		{
			name: "apps comm missing nul",
			edit: func(buf []byte) {
				buf[appsItemStart+appsCommOff+2] = 'x'
			},
			want: ErrMissingNul,
		},
		{
			name: "apps path overlaps comm",
			edit: func(buf []byte) {
				ne.PutUint32(buf[appsItemStart+40:appsItemStart+44], uint32(appsCommOff))
			},
			want: ErrBadLayout,
		},
		{
			name: "apps name missing nul",
			edit: func(buf []byte) {
				buf[appsItemStart+appsNameOff+1] = 'x'
			},
			want: ErrMissingNul,
		},
		{
			name: "apps bogus label count",
			edit: func(buf []byte) {
				ne.PutUint16(buf[appsItemStart+56:appsItemStart+58], 1)
			},
			want: ErrOutOfBounds,
		},
		{
			name: "apps path interior nul",
			edit: func(buf []byte) {
				buf[appsItemStart+appsPathOff] = 0
			},
			want: ErrBadLayout,
		},
	} {
		bad := append([]byte(nil), apps[:appsLen]...)
		tc.edit(bad)
		if _, err := DecodeAppsLookupResponse(bad); !errors.Is(err, tc.want) {
			t.Fatalf("%s error = %v, want %v", tc.name, err, tc.want)
		}
	}

	var cgroups [256]byte
	cgroupsBuilder := NewCgroupsLookupBuilder(cgroups[:], 1, 2)
	if err := cgroupsBuilder.Add(CgroupLookupKnown, OrchestratorK8s, []byte("/p"), []byte("n"), nil); err != nil {
		t.Fatalf("add cgroups known item: %v", err)
	}
	cgroupsLen := cgroupsBuilder.Finish()
	cgroupsItemStart := cgroupsLookupItemStart(t, cgroups[:cgroupsLen])
	cgroupsItem := cgroups[cgroupsItemStart:cgroupsLen]
	cgroupsPathOff := int(ne.Uint32(cgroupsItem[8:12]))
	cgroupsNameOff := int(ne.Uint32(cgroupsItem[16:20]))

	for _, tc := range []struct {
		name string
		edit func([]byte)
		want error
	}{
		{
			name: "cgroups path offset before header",
			edit: func(buf []byte) {
				ne.PutUint32(buf[cgroupsItemStart+8:cgroupsItemStart+12], CgroupsLookupItemHdr-1)
			},
			want: ErrOutOfBounds,
		},
		{
			name: "cgroups path missing nul",
			edit: func(buf []byte) {
				buf[cgroupsItemStart+cgroupsPathOff+2] = 'x'
			},
			want: ErrMissingNul,
		},
		{
			name: "cgroups name overlaps path",
			edit: func(buf []byte) {
				ne.PutUint32(buf[cgroupsItemStart+16:cgroupsItemStart+20], uint32(cgroupsPathOff+2))
				ne.PutUint32(buf[cgroupsItemStart+20:cgroupsItemStart+24], 0)
			},
			want: ErrBadLayout,
		},
		{
			name: "cgroups name missing nul",
			edit: func(buf []byte) {
				buf[cgroupsItemStart+cgroupsNameOff+1] = 'x'
			},
			want: ErrMissingNul,
		},
		{
			name: "cgroups bogus label count",
			edit: func(buf []byte) {
				ne.PutUint16(buf[cgroupsItemStart+24:cgroupsItemStart+26], 1)
			},
			want: ErrOutOfBounds,
		},
		{
			name: "cgroups path interior nul",
			edit: func(buf []byte) {
				buf[cgroupsItemStart+cgroupsPathOff] = 0
			},
			want: ErrBadLayout,
		},
	} {
		bad := append([]byte(nil), cgroups[:cgroupsLen]...)
		tc.edit(bad)
		if _, err := DecodeCgroupsLookupResponse(bad); !errors.Is(err, tc.want) {
			t.Fatalf("%s error = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestLookupMalformedUnknownItemsRejectFieldCorruption(t *testing.T) {
	var apps [256]byte
	appsBuilder := NewAppsLookupBuilder(apps[:], 1, 3)
	if err := appsBuilder.Add(PidLookupUnknown, 0, 0, 4321, 0, NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
		t.Fatalf("add apps unknown item: %v", err)
	}
	appsLen := appsBuilder.Finish()
	appsItemStart := appsLookupItemStart(t, apps[:appsLen])
	appsItem := apps[appsItemStart:appsLen]
	appsCommOff := int(ne.Uint32(appsItem[32:36]))
	appsNameOff := int(ne.Uint32(appsItem[48:52]))

	for _, tc := range []struct {
		name string
		edit func([]byte)
		want error
	}{
		{
			name: "apps unknown comm missing nul",
			edit: func(buf []byte) {
				buf[appsItemStart+appsCommOff] = 'x'
			},
			want: ErrMissingNul,
		},
		{
			name: "apps unknown name offset out of bounds",
			edit: func(buf []byte) {
				ne.PutUint32(buf[appsItemStart+48:appsItemStart+52], uint32(len(appsItem)+1))
			},
			want: ErrOutOfBounds,
		},
		{
			name: "apps unknown overlapping empty strings",
			edit: func(buf []byte) {
				ne.PutUint32(buf[appsItemStart+40:appsItemStart+44], uint32(appsCommOff))
				ne.PutUint32(buf[appsItemStart+48:appsItemStart+52], uint32(appsNameOff))
			},
			want: ErrBadLayout,
		},
	} {
		bad := append([]byte(nil), apps[:appsLen]...)
		tc.edit(bad)
		if _, err := DecodeAppsLookupResponse(bad); !errors.Is(err, tc.want) {
			t.Fatalf("%s error = %v, want %v", tc.name, err, tc.want)
		}
	}

	var cgroups [256]byte
	cgroupsBuilder := NewCgroupsLookupBuilder(cgroups[:], 1, 4)
	if err := cgroupsBuilder.Add(CgroupLookupUnknownRetryLater, 0, []byte("/x"), nil, nil); err != nil {
		t.Fatalf("add cgroups unknown item: %v", err)
	}
	cgroupsLen := cgroupsBuilder.Finish()
	cgroupsItemStart := cgroupsLookupItemStart(t, cgroups[:cgroupsLen])
	cgroupsItem := cgroups[cgroupsItemStart:cgroupsLen]
	cgroupsPathOff := int(ne.Uint32(cgroupsItem[8:12]))
	cgroupsNameOff := int(ne.Uint32(cgroupsItem[16:20]))

	for _, tc := range []struct {
		name string
		edit func([]byte)
		want error
	}{
		{
			name: "cgroups unknown path missing nul",
			edit: func(buf []byte) {
				buf[cgroupsItemStart+cgroupsPathOff+2] = 'x'
			},
			want: ErrMissingNul,
		},
		{
			name: "cgroups unknown name offset out of bounds",
			edit: func(buf []byte) {
				ne.PutUint32(buf[cgroupsItemStart+16:cgroupsItemStart+20], uint32(len(cgroupsItem)+1))
			},
			want: ErrOutOfBounds,
		},
		{
			name: "cgroups unknown name missing nul",
			edit: func(buf []byte) {
				buf[cgroupsItemStart+cgroupsNameOff] = 'x'
			},
			want: ErrMissingNul,
		},
	} {
		bad := append([]byte(nil), cgroups[:cgroupsLen]...)
		tc.edit(bad)
		if _, err := DecodeCgroupsLookupResponse(bad); !errors.Is(err, tc.want) {
			t.Fatalf("%s error = %v, want %v", tc.name, err, tc.want)
		}
	}

	badAppsTrailing := append([]byte(nil), apps[:appsLen]...)
	badAppsTrailing = append(badAppsTrailing, 0)
	appsDirLen := ne.Uint32(badAppsTrailing[AppsLookupRespHdr+4 : AppsLookupRespHdr+8])
	ne.PutUint32(badAppsTrailing[AppsLookupRespHdr+4:AppsLookupRespHdr+8], appsDirLen+1)
	if _, err := DecodeAppsLookupResponse(badAppsTrailing); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("apps unknown trailing data error = %v, want ErrBadLayout", err)
	}

	badCgroupsTrailing := append([]byte(nil), cgroups[:cgroupsLen]...)
	badCgroupsTrailing = append(badCgroupsTrailing, 0)
	cgroupsDirLen := ne.Uint32(badCgroupsTrailing[CgroupsLookupRespHdr+4 : CgroupsLookupRespHdr+8])
	ne.PutUint32(badCgroupsTrailing[CgroupsLookupRespHdr+4:CgroupsLookupRespHdr+8], cgroupsDirLen+1)
	if _, err := DecodeCgroupsLookupResponse(badCgroupsTrailing); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("cgroups unknown trailing data error = %v, want ErrBadLayout", err)
	}
}

func TestLookupBuildersMarkOversizedAndPayloadExceededSuffixes(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		var firstBuf [300]byte
		builder := NewAppsLookupBuilder(firstBuf[:], 3, 77)
		if err := builder.Add(
			PidLookupKnown,
			AppsCgroupKnown,
			OrchestratorDocker,
			100,
			1,
			1000,
			10,
			[]byte("ok"),
			[]byte("/short"),
			[]byte("n"),
			nil,
		); err != nil {
			t.Fatalf("add first apps item: %v", err)
		}
		if err := builder.Add(
			PidLookupKnown,
			AppsCgroupKnown,
			OrchestratorDocker,
			101,
			1,
			1000,
			11,
			[]byte("ok"),
			bytes.Repeat([]byte("x"), len(firstBuf)),
			[]byte("n"),
			nil,
		); err != nil {
			t.Fatalf("add overflowing apps item: %v", err)
		}
		if err := builder.Add(
			PidLookupKnown,
			AppsCgroupKnown,
			OrchestratorDocker,
			102,
			1,
			1000,
			12,
			[]byte("ok"),
			[]byte("/short"),
			[]byte("n"),
			nil,
		); err != nil {
			t.Fatalf("add suffix apps item: %v", err)
		}
		total := builder.Finish()
		view, err := DecodeAppsLookupResponse(firstBuf[:total])
		if err != nil {
			t.Fatalf("decode apps payload-exceeded response: %v", err)
		}
		for i, want := range []struct {
			pid    uint32
			status uint16
		}{
			{100, PidLookupKnown},
			{101, PidLookupPayloadExceeded},
			{102, PidLookupPayloadExceeded},
		} {
			item, err := view.Item(uint32(i))
			if err != nil {
				t.Fatalf("apps item %d: %v", i, err)
			}
			if item.Pid != want.pid || item.Status != want.status {
				t.Fatalf("apps item %d = pid/status %d/%d, want %d/%d", i, item.Pid, item.Status, want.pid, want.status)
			}
		}

		var aloneBuf [AppsLookupRespHdr + LookupDirEntrySize + appsLookupUnknownItemSize]byte
		alone := NewAppsLookupBuilder(aloneBuf[:], 1, 78)
		if err := alone.Add(
			PidLookupKnown,
			AppsCgroupKnown,
			OrchestratorDocker,
			200,
			1,
			1000,
			20,
			[]byte("ok"),
			bytes.Repeat([]byte("y"), len(aloneBuf)),
			[]byte("n"),
			nil,
		); err != nil {
			t.Fatalf("add single oversized apps item: %v", err)
		}
		total = alone.Finish()
		view, err = DecodeAppsLookupResponse(aloneBuf[:total])
		if err != nil {
			t.Fatalf("decode apps oversized response: %v", err)
		}
		item, err := view.Item(0)
		if err != nil {
			t.Fatalf("apps oversized item: %v", err)
		}
		if item.Pid != 200 || item.Status != PidLookupOversizedItem {
			t.Fatalf("apps oversized item = pid/status %d/%d", item.Pid, item.Status)
		}
	})

	t.Run("cgroups", func(t *testing.T) {
		var firstBuf [176]byte
		builder := NewCgroupsLookupBuilder(firstBuf[:], 3, 88)
		if err := builder.Add(CgroupLookupKnown, OrchestratorK8s, []byte("/a"), []byte("ok"), nil); err != nil {
			t.Fatalf("add first cgroups item: %v", err)
		}
		if err := builder.Add(CgroupLookupKnown, OrchestratorK8s, []byte("/b"), bytes.Repeat([]byte("x"), len(firstBuf)), nil); err != nil {
			t.Fatalf("add overflowing cgroups item: %v", err)
		}
		if err := builder.Add(CgroupLookupKnown, OrchestratorK8s, []byte("/c"), []byte("ok"), nil); err != nil {
			t.Fatalf("add suffix cgroups item: %v", err)
		}
		total := builder.Finish()
		view, err := DecodeCgroupsLookupResponse(firstBuf[:total])
		if err != nil {
			t.Fatalf("decode cgroups payload-exceeded response: %v", err)
		}
		for i, want := range []struct {
			path   string
			status uint16
		}{
			{"/a", CgroupLookupKnown},
			{"/b", CgroupLookupPayloadExceeded},
			{"/c", CgroupLookupPayloadExceeded},
		} {
			item, err := view.Item(uint32(i))
			if err != nil {
				t.Fatalf("cgroups item %d: %v", i, err)
			}
			if item.Path.String() != want.path || item.Status != want.status {
				t.Fatalf("cgroups item %d = path/status %q/%d, want %q/%d", i, item.Path.String(), item.Status, want.path, want.status)
			}
		}

		var aloneBuf [CgroupsLookupRespHdr + LookupDirEntrySize + CgroupsLookupItemHdr + 32]byte
		alone := NewCgroupsLookupBuilder(aloneBuf[:], 1, 89)
		if err := alone.Add(CgroupLookupKnown, OrchestratorK8s, []byte("/huge"), bytes.Repeat([]byte("y"), len(aloneBuf)), nil); err != nil {
			t.Fatalf("add single oversized cgroups item: %v", err)
		}
		total = alone.Finish()
		view, err = DecodeCgroupsLookupResponse(aloneBuf[:total])
		if err != nil {
			t.Fatalf("decode cgroups oversized response: %v", err)
		}
		item, err := view.Item(0)
		if err != nil {
			t.Fatalf("cgroups oversized item: %v", err)
		}
		if item.Path.String() != "/huge" || item.Status != CgroupLookupOversizedItem {
			t.Fatalf("cgroups oversized item = path/status %q/%d", item.Path.String(), item.Status)
		}
	})
}

func TestLookupBuilderCapacityAndLabelGuards(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		zero := NewAppsLookupBuilder(make([]byte, AppsLookupRespHdr), 0, 0)
		if err := zero.Add(PidLookupUnknown, 0, 0, 1, 0, NipcUIDUnset, 0, nil, nil, nil, nil); !errors.Is(err, ErrOverflow) {
			t.Fatalf("apps max-items guard = %v, want ErrOverflow", err)
		}

		noPayload := NewAppsLookupBuilder(make([]byte, AppsLookupRespHdr+LookupDirEntrySize), 1, 0)
		if err := noPayload.Add(PidLookupUnknown, 0, 0, 2, 0, NipcUIDUnset, 0, nil, nil, nil, nil); !errors.Is(err, ErrOverflow) {
			t.Fatalf("apps unknown with no payload room = %v, want ErrOverflow", err)
		}

		var buf [512]byte
		builder := NewAppsLookupBuilder(buf[:], 1, 0)
		err := builder.Add(
			PidLookupKnown,
			AppsCgroupKnown,
			OrchestratorDocker,
			3,
			0,
			0,
			1,
			[]byte("ok"),
			[]byte("/cg"),
			[]byte("name"),
			[]struct{ Key, Value []byte }{{Key: nil, Value: []byte("bad")}},
		)
		if !errors.Is(err, ErrBadLayout) {
			t.Fatalf("apps invalid label source = %v, want ErrBadLayout", err)
		}
	})

	t.Run("cgroups", func(t *testing.T) {
		zero := NewCgroupsLookupBuilder(make([]byte, CgroupsLookupRespHdr), 0, 0)
		if err := zero.Add(CgroupLookupUnknownRetryLater, 0, []byte("/a"), nil, nil); !errors.Is(err, ErrOverflow) {
			t.Fatalf("cgroups max-items guard = %v, want ErrOverflow", err)
		}

		noPayload := NewCgroupsLookupBuilder(make([]byte, CgroupsLookupRespHdr+LookupDirEntrySize), 1, 0)
		if err := noPayload.Add(CgroupLookupUnknownRetryLater, 0, []byte("/b"), nil, nil); !errors.Is(err, ErrOverflow) {
			t.Fatalf("cgroups unknown with no payload room = %v, want ErrOverflow", err)
		}

		var buf [256]byte
		builder := NewCgroupsLookupBuilder(buf[:], 1, 0)
		err := builder.Add(
			CgroupLookupKnown,
			OrchestratorK8s,
			[]byte("/c"),
			[]byte("name"),
			[]struct{ Key, Value []byte }{{Key: []byte("bad\x00key"), Value: []byte("value")}},
		)
		if !errors.Is(err, ErrBadLayout) {
			t.Fatalf("cgroups invalid label source = %v, want ErrBadLayout", err)
		}
	})
}

func TestLookupBuildersRejectItemOffsetsAboveWireRange(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("synthetic 32-bit wire offset guards require 64-bit int")
	}

	maxU32Value := ^uint32(0)
	largeOffset := int(uint64(maxU32Value) + 8)

	appsKnown := &AppsLookupBuilder{buf: make([]byte, AppsLookupRespHdr+LookupDirEntrySize), maxItems: 1, dataOffset: largeOffset}
	if err := appsKnown.Add(PidLookupKnown, AppsCgroupHostRoot, 0, 1, 0, 0, 0, []byte("x"), nil, nil, nil); !errors.Is(err, ErrOverflow) {
		t.Fatalf("apps known high item offset error = %v, want ErrOverflow", err)
	}

	appsUnknown := &AppsLookupBuilder{buf: make([]byte, AppsLookupRespHdr+LookupDirEntrySize), maxItems: 1, dataOffset: largeOffset}
	if err := appsUnknown.Add(PidLookupUnknown, 0, 0, 2, 0, NipcUIDUnset, 0, nil, nil, nil, nil); !errors.Is(err, ErrOverflow) {
		t.Fatalf("apps unknown high item offset error = %v, want ErrOverflow", err)
	}

	cgroupsKnown := &CgroupsLookupBuilder{buf: make([]byte, CgroupsLookupRespHdr+LookupDirEntrySize), maxItems: 1, dataOffset: largeOffset}
	if err := cgroupsKnown.Add(CgroupLookupKnown, OrchestratorK8s, []byte("/x"), nil, nil); !errors.Is(err, ErrOverflow) {
		t.Fatalf("cgroups known high item offset error = %v, want ErrOverflow", err)
	}

	cgroupsUnknown := &CgroupsLookupBuilder{buf: make([]byte, CgroupsLookupRespHdr+LookupDirEntrySize), maxItems: 1, dataOffset: largeOffset}
	if err := cgroupsUnknown.Add(CgroupLookupUnknownRetryLater, 0, []byte("/x"), nil, nil); !errors.Is(err, ErrOverflow) {
		t.Fatalf("cgroups unknown high item offset error = %v, want ErrOverflow", err)
	}
}

func TestLookupRejectsUnrepresentableItemCounts(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("oversized slice-header guard requires 64-bit int")
	}
	maxU32Value := ^uint32(0)
	hugeLen := int(uint64(maxU32Value) + 1)

	if _, _, ok := appsLookupRequestLayoutForCount(hugeLen); ok {
		t.Fatal("huge apps request count should overflow")
	}

	if _, ok := cgroupsLookupRequestPackedStartForCount(hugeLen); ok {
		t.Fatal("huge cgroups request count should overflow")
	}

	if _, _, ok := lookupRawResponseMinBytes(AppsLookupRespHdr, hugeLen); ok {
		t.Fatal("huge apps raw response count should overflow")
	}
	if _, _, ok := lookupRawResponseMinBytes(CgroupsLookupRespHdr, hugeLen); ok {
		t.Fatal("huge cgroups raw response count should overflow")
	}
}

func TestLookupViewsRejectCorruptDirectPayloads(t *testing.T) {
	appsReqPayload := make([]byte, AppsLookupReqHdr+LookupDirEntrySize)
	ne.PutUint32(appsReqPayload[AppsLookupReqHdr+4:AppsLookupReqHdr+8], AppsLookupKeySize)
	appsReq := &AppsLookupRequestView{ItemCount: 1, payload: appsReqPayload}
	if _, err := appsReq.Item(0); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("corrupt apps request item = %v, want ErrOutOfBounds", err)
	}

	appsRespPayload := make([]byte, AppsLookupRespHdr+LookupDirEntrySize)
	ne.PutUint32(appsRespPayload[AppsLookupRespHdr+4:AppsLookupRespHdr+8], AppsLookupItemHdr)
	appsResp := &AppsLookupResponseView{ItemCount: 1, payload: appsRespPayload}
	if _, err := appsResp.Item(0); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("corrupt apps response item = %v, want ErrOutOfBounds", err)
	}

	cgroupsReqPayload := make([]byte, CgroupsLookupReqHdr+LookupDirEntrySize)
	cgroupsReq := &CgroupsLookupRequestView{ItemCount: 1, payload: cgroupsReqPayload}
	if _, err := cgroupsReq.Item(0); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("corrupt cgroups request item = %v, want ErrOutOfBounds", err)
	}

	cgroupsRespPayload := make([]byte, CgroupsLookupRespHdr+LookupDirEntrySize)
	ne.PutUint32(cgroupsRespPayload[CgroupsLookupRespHdr+4:CgroupsLookupRespHdr+8], CgroupsLookupItemHdr)
	cgroupsResp := &CgroupsLookupResponseView{ItemCount: 1, payload: cgroupsRespPayload}
	if _, err := cgroupsResp.Item(0); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("corrupt cgroups response item = %v, want ErrOutOfBounds", err)
	}
}

func TestLookupCommonHelperErrorGuards(t *testing.T) {
	if strconv.IntSize >= 64 {
		maxU32Value := ^uint32(0)
		overU32 := int(uint64(maxU32Value) + 1)
		if err := validateLookupDir(make([]byte, 16), 0, 0, 0, overU32, -1); !errors.Is(err, ErrBadLayout) {
			t.Fatalf("validateLookupDir minLen conversion error = %v, want ErrBadLayout", err)
		}
		if err := validateLookupDir(make([]byte, 16), 0, 0, 0, -1, overU32); !errors.Is(err, ErrBadLayout) {
			t.Fatalf("validateLookupDir exactLen conversion error = %v, want ErrBadLayout", err)
		}
	}

	padding := make([]byte, 24)
	padding[5] = 1
	if _, err := validateLabels(padding, 4, 1, 5); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("label padding error = %v, want ErrBadLayout", err)
	}

	zeroKey := make([]byte, 24)
	ne.PutUint32(zeroKey[8:12], 24)
	if _, err := validateLabels(zeroKey, 4, 1, 4); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("zero label key error = %v, want ErrBadLayout", err)
	}

	wrongValueOffset := make([]byte, 29)
	ne.PutUint32(wrongValueOffset[8:12], 24)
	ne.PutUint32(wrongValueOffset[12:16], 1)
	ne.PutUint32(wrongValueOffset[16:20], 27)
	ne.PutUint32(wrongValueOffset[20:24], 1)
	wrongValueOffset[24] = 'k'
	wrongValueOffset[25] = 0
	wrongValueOffset[26] = 'v'
	wrongValueOffset[27] = 0
	if _, err := validateLabels(wrongValueOffset, 4, 1, 4); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("wrong label value offset error = %v, want ErrBadLayout", err)
	}

	trailing := make([]byte, 29)
	ne.PutUint32(trailing[8:12], 24)
	ne.PutUint32(trailing[12:16], 1)
	ne.PutUint32(trailing[16:20], 26)
	ne.PutUint32(trailing[20:24], 1)
	trailing[24] = 'k'
	trailing[25] = 0
	trailing[26] = 'v'
	trailing[27] = 0
	if _, err := validateLabels(trailing, 4, 1, 4); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("label trailing data error = %v, want ErrBadLayout", err)
	}

	if _, err := lookupLabelAt(trailing, 4, 2, maxIntValue()-7, 1); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("lookupLabelAt base overflow = %v, want ErrOutOfBounds", err)
	}
	if _, err := writeLookupLabels(make([]byte, 64), maxIntValue(), 16, []struct{ Key, Value []byte }{{Key: []byte("k")}}); !errors.Is(err, ErrOverflow) {
		t.Fatalf("writeLookupLabels next overflow = %v, want ErrOverflow", err)
	}
	if _, _, _, err := labelLayoutGo(maxIntValue(), []struct{ Key, Value []byte }{{Key: []byte("k")}}); !errors.Is(err, ErrOverflow) {
		t.Fatalf("labelLayoutGo align overflow = %v, want ErrOverflow", err)
	}
	corruptFinish := make([]byte, AppsLookupRespHdr+LookupDirEntrySize)
	ne.PutUint32(corruptFinish[AppsLookupRespHdr:AppsLookupRespHdr+4], 8)
	if total := finishLookupResponse(corruptFinish, AppsLookupRespHdr, 1, 0, 0); total != 0 {
		t.Fatalf("finishLookupResponse corrupt first offset = %d, want 0", total)
	}

	short := make([]byte, AppsLookupRespHdr+LookupDirEntrySize)
	if n, err := encodeLookupRawResponse(short, AppsLookupRespHdr, 0, [][]byte{{1, 2, 3}}, AppsLookupItemHdr, nil); n != 0 || !errors.Is(err, ErrBadLayout) {
		t.Fatalf("raw response short item = n %d err %v, want 0 ErrBadLayout", n, err)
	}
	if n, err := encodeLookupRawResponse(short, AppsLookupRespHdr, 0, [][]byte{make([]byte, AppsLookupItemHdr)}, AppsLookupItemHdr, func([]byte) error {
		return ErrBadLayout
	}); n != 0 || !errors.Is(err, ErrBadLayout) {
		t.Fatalf("raw response validate error = n %d err %v, want 0 ErrBadLayout", n, err)
	}

	if strconv.IntSize >= 64 {
		if _, _, _, _, _, _, ok := lookupLabelWriteLayout(16, maxIntValue(), 0); ok {
			t.Fatal("writeLookupLabels key length should overflow")
		}
		_, dataOffset, ok := lookupRawResponseMinBytes(AppsLookupRespHdr, 1)
		if !ok {
			t.Fatal("one-item raw response minimum should fit")
		}
		if _, _, ok := lookupRawResponseItemBounds(dataOffset, maxIntValue()); ok {
			t.Fatal("raw response huge item should overflow")
		}
	}
}

func TestLookupAdditionalBoundaryGuards(t *testing.T) {
	expectPanic := func(name string, f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatalf("%s did not panic", name)
			}
		}()
		f()
	}

	expectPanic("apps builder short buffer", func() {
		NewAppsLookupBuilder(make([]byte, AppsLookupRespHdr), 1, 0)
	})
	expectPanic("cgroups builder short buffer", func() {
		NewCgroupsLookupBuilder(make([]byte, CgroupsLookupRespHdr), 1, 0)
	})

	if _, err := lookupPayloadSlice(make([]byte, 8), maxIntValue(), 1, 0); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("lookupPayloadSlice start overflow = %v, want ErrOutOfBounds", err)
	}
	if err := validateLookupDir(make([]byte, 8), maxIntValue(), 1, 0, 0, -1); !errors.Is(err, ErrBadItemCount) {
		t.Fatalf("validateLookupDir directory-end overflow = %v, want ErrBadItemCount", err)
	}

	unaligned := make([]byte, 8)
	ne.PutUint32(unaligned[0:4], 1)
	ne.PutUint32(unaligned[4:8], 1)
	if err := validateLookupDir(unaligned, 0, 1, 8, 0, -1); !errors.Is(err, ErrBadAlignment) {
		t.Fatalf("validateLookupDir unaligned offset = %v, want ErrBadAlignment", err)
	}

	overlapDir := make([]byte, 16)
	ne.PutUint32(overlapDir[0:4], 8)
	ne.PutUint32(overlapDir[4:8], 8)
	ne.PutUint32(overlapDir[8:12], 0)
	ne.PutUint32(overlapDir[12:16], 8)
	if err := validateLookupDir(overlapDir, 0, 2, 32, 0, -1); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("validateLookupDir overlapping entries = %v, want ErrBadLayout", err)
	}

	keyBeforeHeader := make([]byte, 24)
	ne.PutUint32(keyBeforeHeader[4:8], 3)
	if _, err := lookupLabelAt(keyBeforeHeader, 4, 1, 4, 0); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("lookupLabelAt key before header = %v, want ErrOutOfBounds", err)
	}

	valueBeforeHeader := make([]byte, 24)
	ne.PutUint32(valueBeforeHeader[4:8], 20)
	ne.PutUint32(valueBeforeHeader[8:12], 1)
	ne.PutUint32(valueBeforeHeader[12:16], 3)
	valueBeforeHeader[20] = 'k'
	valueBeforeHeader[21] = 0
	if _, err := lookupLabelAt(valueBeforeHeader, 4, 1, 4, 0); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("lookupLabelAt value before header = %v, want ErrOutOfBounds", err)
	}

	valueMissingNul := make([]byte, 24)
	ne.PutUint32(valueMissingNul[4:8], 20)
	ne.PutUint32(valueMissingNul[8:12], 1)
	ne.PutUint32(valueMissingNul[12:16], 22)
	ne.PutUint32(valueMissingNul[16:20], 1)
	valueMissingNul[20] = 'k'
	valueMissingNul[21] = 0
	valueMissingNul[22] = 'v'
	valueMissingNul[23] = 'x'
	if _, err := lookupLabelAt(valueMissingNul, 4, 1, 4, 0); !errors.Is(err, ErrMissingNul) {
		t.Fatalf("lookupLabelAt value missing nul = %v, want ErrMissingNul", err)
	}

	if n, err := EncodeCgroupsLookupRequest([][]byte{nil}, make([]byte, 64)); n != 0 || !errors.Is(err, ErrBadLayout) {
		t.Fatalf("cgroups nil path request = n %d err %v, want 0 ErrBadLayout", n, err)
	}
	if n, err := EncodeCgroupsLookupRequest([][]byte{[]byte("bad\x00path")}, make([]byte, 64)); n != 0 || !errors.Is(err, ErrBadLayout) {
		t.Fatalf("cgroups interior-nul request = n %d err %v, want 0 ErrBadLayout", n, err)
	}
}

func TestLookupBuildersRejectHugeLabelCounts(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("synthetic huge label-count guard requires 64-bit int")
	}

	hugeLabels := make([]struct{ Key, Value []byte }, int(^uint16(0))+1)

	var appsBuf [AppsLookupRespHdr + LookupDirEntrySize + appsLookupUnknownItemSize]byte
	apps := NewAppsLookupBuilder(appsBuf[:], 1, 0)
	if err := apps.Add(
		PidLookupKnown,
		AppsCgroupKnown,
		OrchestratorDocker,
		123,
		0,
		1000,
		1,
		[]byte("ok"),
		[]byte("/path"),
		[]byte("name"),
		hugeLabels,
	); err != nil {
		t.Fatalf("apps huge-label item should become item outcome, got %v", err)
	}
	appsView, err := DecodeAppsLookupResponse(appsBuf[:apps.Finish()])
	if err != nil {
		t.Fatalf("decode apps huge-label response: %v", err)
	}
	appsItem, err := appsView.Item(0)
	if err != nil {
		t.Fatalf("apps huge-label item: %v", err)
	}
	if appsItem.Status != PidLookupOversizedItem {
		t.Fatalf("apps huge-label status = %d, want OVERSIZED_ITEM", appsItem.Status)
	}

	var cgroupsBuf [CgroupsLookupRespHdr + LookupDirEntrySize + cgroupsLookupUnknownFixedBytes + 8]byte
	cgroups := NewCgroupsLookupBuilder(cgroupsBuf[:], 1, 0)
	if err := cgroups.Add(
		CgroupLookupKnown,
		OrchestratorK8s,
		[]byte("/path"),
		[]byte("name"),
		hugeLabels,
	); err != nil {
		t.Fatalf("cgroups huge-label item should become item outcome, got %v", err)
	}
	cgroupsView, err := DecodeCgroupsLookupResponse(cgroupsBuf[:cgroups.Finish()])
	if err != nil {
		t.Fatalf("decode cgroups huge-label response: %v", err)
	}
	cgroupsItem, err := cgroupsView.Item(0)
	if err != nil {
		t.Fatalf("cgroups huge-label item: %v", err)
	}
	if cgroupsItem.Status != CgroupLookupOversizedItem {
		t.Fatalf("cgroups huge-label status = %d, want OVERSIZED_ITEM", cgroupsItem.Status)
	}
}

func TestCgroupsSnapshotAdditionalGuardCoverage(t *testing.T) {
	expectPanic := func(name string, f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatalf("%s did not panic", name)
			}
		}()
		f()
	}

	if _, err := DecodeCgroupsResponse(make([]byte, cgroupsRespHdr-1)); !errors.Is(err, ErrTruncated) {
		t.Fatalf("short snapshot response = %v, want ErrTruncated", err)
	}

	badLayout := make([]byte, cgroupsRespHdr)
	ne.PutUint16(badLayout[0:2], 2)
	if _, err := DecodeCgroupsResponse(badLayout); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("snapshot bad layout version = %v, want ErrBadLayout", err)
	}

	badFlags := make([]byte, cgroupsRespHdr)
	ne.PutUint16(badFlags[0:2], 1)
	ne.PutUint16(badFlags[2:4], 1)
	if _, err := DecodeCgroupsResponse(badFlags); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("snapshot bad flags = %v, want ErrBadLayout", err)
	}

	badReserved := make([]byte, cgroupsRespHdr)
	ne.PutUint16(badReserved[0:2], 1)
	ne.PutUint32(badReserved[12:16], 1)
	if _, err := DecodeCgroupsResponse(badReserved); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("snapshot bad reserved = %v, want ErrBadLayout", err)
	}

	unaligned := make([]byte, cgroupsRespHdr+cgroupsDirEntry+cgroupsItemHdr)
	ne.PutUint16(unaligned[0:2], 1)
	ne.PutUint32(unaligned[4:8], 1)
	ne.PutUint32(unaligned[cgroupsRespHdr:cgroupsRespHdr+4], 1)
	ne.PutUint32(unaligned[cgroupsRespHdr+4:cgroupsRespHdr+8], cgroupsItemHdr)
	if _, err := DecodeCgroupsResponse(unaligned); !errors.Is(err, ErrBadAlignment) {
		t.Fatalf("snapshot unaligned directory = %v, want ErrBadAlignment", err)
	}

	outOfBounds := make([]byte, cgroupsRespHdr+cgroupsDirEntry)
	ne.PutUint16(outOfBounds[0:2], 1)
	ne.PutUint32(outOfBounds[4:8], 1)
	ne.PutUint32(outOfBounds[cgroupsRespHdr+4:cgroupsRespHdr+8], cgroupsItemHdr)
	if _, err := DecodeCgroupsResponse(outOfBounds); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("snapshot out-of-bounds item = %v, want ErrOutOfBounds", err)
	}

	shortItem := make([]byte, cgroupsRespHdr+cgroupsDirEntry+cgroupsItemHdr-1)
	ne.PutUint16(shortItem[0:2], 1)
	ne.PutUint32(shortItem[4:8], 1)
	ne.PutUint32(shortItem[cgroupsRespHdr+4:cgroupsRespHdr+8], cgroupsItemHdr-1)
	if _, err := DecodeCgroupsResponse(shortItem); !errors.Is(err, ErrTruncated) {
		t.Fatalf("snapshot short item = %v, want ErrTruncated", err)
	}

	expectPanic("snapshot builder short buffer", func() {
		NewCgroupsBuilder(make([]byte, cgroupsRespHdr), 1, 0, 0)
	})

	zero := NewCgroupsBuilder(make([]byte, cgroupsRespHdr), 0, 0, 7)
	if total := zero.Finish(); total != cgroupsRespHdr {
		t.Fatalf("zero-item snapshot finish = %d, want %d", total, cgroupsRespHdr)
	}
	zeroView, err := DecodeCgroupsResponse(zero.buf[:cgroupsRespHdr])
	if err != nil {
		t.Fatalf("decode zero-item snapshot: %v", err)
	}
	if zeroView.ItemCount != 0 || zeroView.Generation != 7 {
		t.Fatalf("zero-item snapshot view = count/generation %d/%d", zeroView.ItemCount, zeroView.Generation)
	}

	if strconv.IntSize < 64 {
		t.Skip("synthetic snapshot overflow guards require 64-bit int")
	}

	maxU32Value := ^uint32(0)
	tooLarge := int(uint64(maxU32Value) + 1)
	maxU32Bytes := int(maxU32Value)

	if _, _, _, ok := cgroupsItemLayoutForLengths(tooLarge, 0); ok {
		t.Fatal("snapshot huge name layout should overflow")
	}
	if _, _, _, ok := cgroupsItemLayoutForLengths(0, tooLarge); ok {
		t.Fatal("snapshot huge path layout should overflow")
	}
	if _, _, _, ok := cgroupsItemLayoutForLengths(maxU32Bytes, len("p")); ok {
		t.Fatal("snapshot path-offset layout should overflow")
	}
	if _, _, _, ok := cgroupsItemLayoutForLengths(0, maxU32Bytes); ok {
		t.Fatal("snapshot item-size layout should overflow")
	}
	if err := (&CgroupsBuilder{buf: make([]byte, cgroupsRespHdr+cgroupsDirEntry), maxItems: 1, dataOffset: maxIntValue()}).Add(1, 0, 1, []byte("n"), []byte("p")); !errors.Is(err, ErrOverflow) {
		t.Fatalf("snapshot data-offset overflow error = %v, want ErrOverflow", err)
	}
}

func TestProtocolFinalizerAndDispatchGuards(t *testing.T) {
	var request [cgroupsReqSize]byte
	(&CgroupsRequest{LayoutVersion: 1}).Encode(request[:])
	if n, ok := DispatchCgroupsSnapshot(request[:], make([]byte, cgroupsRespHdr), 1, func(*CgroupsRequest, *CgroupsBuilder) bool {
		return true
	}); n != 0 || ok {
		t.Fatalf("snapshot dispatch short response = n %d ok %v, want 0/false", n, ok)
	}

	finishOverflow := &CgroupsBuilder{
		buf:        make([]byte, 128),
		itemCount:  1,
		dataOffset: 0,
	}
	ne.PutUint32(finishOverflow.buf[cgroupsRespHdr:cgroupsRespHdr+4], 100)
	if total := finishOverflow.Finish(); total != 0 {
		t.Fatalf("snapshot finish negative packed length = %d, want 0", total)
	}

	outOfOrder := &CgroupsBuilder{
		buf:        make([]byte, 128),
		itemCount:  2,
		dataOffset: 64,
	}
	ne.PutUint32(outOfOrder.buf[cgroupsRespHdr:cgroupsRespHdr+4], 64)
	ne.PutUint32(outOfOrder.buf[cgroupsRespHdr+8:cgroupsRespHdr+12], 32)
	if total := outOfOrder.Finish(); total != 0 {
		t.Fatalf("snapshot finish out-of-order directory = %d, want 0", total)
	}

	maxInt := maxIntValue()
	if total, count := (&BatchBuilder{
		buf:        make([]byte, 1),
		itemCount:  0,
		dirEnd:     maxInt,
		dataOffset: 1,
	}).Finish(); total != 0 || count != 0 {
		t.Fatalf("batch finish data-end overflow = total/count %d/%d, want 0/0", total, count)
	}
	if err := (&BatchBuilder{
		buf:        make([]byte, 1),
		maxItems:   1,
		dirEnd:     maxInt,
		dataOffset: 1,
	}).Add(nil); !errors.Is(err, ErrOverflow) {
		t.Fatalf("batch add absolute-position overflow = %v, want ErrOverflow", err)
	}

	if strconv.IntSize >= 64 {
		maxU32Value := ^uint32(0)
		if _, ok := stringReverseEncodedLen(int(uint64(maxU32Value) + 1)); ok {
			t.Fatal("huge string reverse length should overflow")
		}
	}
}

func TestLookupProtocolDispatchHandlerGuards(t *testing.T) {
	var appsReq [128]byte
	appsReqLen, err := EncodeAppsLookupRequest([]uint32{1234}, appsReq[:])
	if err != nil {
		t.Fatalf("encode apps request: %v", err)
	}
	var cgroupsReq [128]byte
	cgroupsReqLen, err := EncodeCgroupsLookupRequest([][]byte{[]byte("/known")}, cgroupsReq[:])
	if err != nil {
		t.Fatalf("encode cgroups request: %v", err)
	}

	t.Run("apps", func(t *testing.T) {
		var resp [256]byte
		n, err := DispatchAppsLookup(appsReq[:appsReqLen], resp[:],
			func(req *AppsLookupRequestView, builder *AppsLookupBuilder) bool {
				pid, ierr := req.Item(0)
				if ierr != nil {
					return false
				}
				return builder.Add(PidLookupUnknown, 0, 0, pid, 0, NipcUIDUnset, 0, nil, nil, nil, nil) == nil
			})
		if err != nil || n == 0 {
			t.Fatalf("apps dispatch success = n %d err %v", n, err)
		}

		n, err = DispatchAppsLookup(appsReq[:appsReqLen], resp[:],
			func(*AppsLookupRequestView, *AppsLookupBuilder) bool { return false })
		if n != 0 || !errors.Is(err, ErrHandlerFailed) {
			t.Fatalf("apps dispatch false = n %d err %v, want 0 ErrHandlerFailed", n, err)
		}

		n, err = DispatchAppsLookup(appsReq[:appsReqLen], resp[:],
			func(_ *AppsLookupRequestView, builder *AppsLookupBuilder) bool {
				_ = builder.Add(PidLookupKnown, AppsCgroupKnown, 0, 1234, 0, 0, 0, []byte("ok"), nil, nil, nil)
				return false
			})
		if n != 0 || !errors.Is(err, ErrBadLayout) {
			t.Fatalf("apps dispatch false builder error = n %d err %v, want 0 ErrBadLayout", n, err)
		}

		n, err = DispatchAppsLookup(appsReq[:appsReqLen], resp[:],
			func(_ *AppsLookupRequestView, builder *AppsLookupBuilder) bool {
				_ = builder.Add(PidLookupKnown, AppsCgroupKnown, 0, 1234, 0, 0, 0, []byte("ok"), nil, nil, nil)
				return true
			})
		if n != 0 || !errors.Is(err, ErrBadLayout) {
			t.Fatalf("apps dispatch true builder error = n %d err %v, want 0 ErrBadLayout", n, err)
		}
	})

	t.Run("cgroups", func(t *testing.T) {
		var resp [256]byte
		n, err := DispatchCgroupsLookup(cgroupsReq[:cgroupsReqLen], resp[:],
			func(req *CgroupsLookupRequestView, builder *CgroupsLookupBuilder) bool {
				path, ierr := req.Item(0)
				if ierr != nil {
					return false
				}
				return builder.Add(CgroupLookupUnknownRetryLater, 0, path.Bytes(), nil, nil) == nil
			})
		if err != nil || n == 0 {
			t.Fatalf("cgroups dispatch success = n %d err %v", n, err)
		}

		n, err = DispatchCgroupsLookup(cgroupsReq[:cgroupsReqLen], resp[:],
			func(*CgroupsLookupRequestView, *CgroupsLookupBuilder) bool { return false })
		if n != 0 || !errors.Is(err, ErrHandlerFailed) {
			t.Fatalf("cgroups dispatch false = n %d err %v, want 0 ErrHandlerFailed", n, err)
		}

		n, err = DispatchCgroupsLookup(cgroupsReq[:cgroupsReqLen], resp[:],
			func(_ *CgroupsLookupRequestView, builder *CgroupsLookupBuilder) bool {
				_ = builder.Add(CgroupLookupKnown, 0, nil, nil, nil)
				return false
			})
		if n != 0 || !errors.Is(err, ErrBadLayout) {
			t.Fatalf("cgroups dispatch false builder error = n %d err %v, want 0 ErrBadLayout", n, err)
		}

		n, err = DispatchCgroupsLookup(cgroupsReq[:cgroupsReqLen], resp[:],
			func(_ *CgroupsLookupRequestView, builder *CgroupsLookupBuilder) bool {
				_ = builder.Add(CgroupLookupKnown, 0, nil, nil, nil)
				return true
			})
		if n != 0 || !errors.Is(err, ErrBadLayout) {
			t.Fatalf("cgroups dispatch true builder error = n %d err %v, want 0 ErrBadLayout", n, err)
		}
	})
}

func TestLookupCommonDirectGuardEdges(t *testing.T) {
	dir := make([]byte, 16)
	ne.PutUint32(dir[0:4], 1)
	ne.PutUint32(dir[4:8], 1)
	if err := validateLookupDir(dir, 0, 1, 16, 1, -1); !errors.Is(err, ErrBadAlignment) {
		t.Fatalf("misaligned lookup dir = %v, want ErrBadAlignment", err)
	}

	ne.PutUint32(dir[0:4], 0)
	ne.PutUint32(dir[4:8], 1)
	if err := validateLookupDir(dir, 0, 1, 16, 2, -1); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("too-small lookup dir item = %v, want ErrBadLayout", err)
	}

	ne.PutUint32(dir[0:4], 8)
	ne.PutUint32(dir[4:8], 9)
	if err := validateLookupDir(dir, 0, 1, 16, 1, -1); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("out-of-bounds lookup dir = %v, want ErrOutOfBounds", err)
	}

	two := make([]byte, 16)
	ne.PutUint32(two[0:4], 8)
	ne.PutUint32(two[4:8], 8)
	ne.PutUint32(two[8:12], 0)
	ne.PutUint32(two[12:16], 8)
	if err := validateLookupDir(two, 0, 2, 16, 1, -1); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("overlapping lookup dir = %v, want ErrBadLayout", err)
	}

	item := append(make([]byte, 16), []byte("ab\x00")...)
	if _, _, err := lookupString(item, 16, 15, 1); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("lookup string before header = %v, want ErrOutOfBounds", err)
	}
	if _, _, err := lookupString([]byte("0123456789abcdefabc"), 16, 16, 3); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("lookup string missing terminator space = %v, want ErrOutOfBounds", err)
	}
	interiorNul := append(make([]byte, 16), []byte{'a', 0, 'b', 0}...)
	if _, _, err := lookupString(interiorNul, 16, 16, 3); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("lookup string interior nul = %v, want ErrBadLayout", err)
	}
	if _, _, err := lookupEmptyString(item, 16, 15); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("empty string before header = %v, want ErrOutOfBounds", err)
	}
	item[16] = 'x'
	if _, _, err := lookupEmptyString(item, 16, 16); !errors.Is(err, ErrMissingNul) {
		t.Fatalf("empty string missing nul = %v, want ErrMissingNul", err)
	}

	if overlap(0, 2, 2, 4) {
		t.Fatal("touching ranges should not overlap")
	}
	if !overlap(0, 3, 2, 4) {
		t.Fatal("intersecting ranges should overlap")
	}
}

func TestLookupLabelGuardEdges(t *testing.T) {
	if _, err := validateLabels(make([]byte, 24), 16, 0, 23); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("zero-label trailing bytes = %v, want ErrBadLayout", err)
	}

	valid := make([]byte, 44)
	ne.PutUint32(valid[24:28], 40)
	ne.PutUint32(valid[28:32], 1)
	ne.PutUint32(valid[32:36], 42)
	ne.PutUint32(valid[36:40], 1)
	valid[40] = 'k'
	valid[41] = 0
	valid[42] = 'v'
	valid[43] = 0
	if table, err := validateLabels(valid, 16, 1, 24); err != nil || table != 24 {
		t.Fatalf("valid labels = table %d err %v, want 24/nil", table, err)
	}
	label, err := lookupLabelAt(valid, 16, 1, 24, 0)
	if err != nil || label.Key.String() != "k" || label.Value.String() != "v" {
		t.Fatalf("lookup label = %+v err %v, want k/v", label, err)
	}
	if _, err := lookupLabelAt(valid, 16, 1, 24, 1); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("lookup label out of bounds = %v, want ErrOutOfBounds", err)
	}

	padded := append([]byte(nil), valid...)
	padded[22] = 1
	if _, err := validateLabels(padded, 16, 1, 22); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("nonzero label padding = %v, want ErrBadLayout", err)
	}

	badKeyLen := append([]byte(nil), valid...)
	ne.PutUint32(badKeyLen[28:32], 0)
	if _, err := validateLabels(badKeyLen, 16, 1, 24); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("empty label key = %v, want ErrBadLayout", err)
	}

	badValueOff := append([]byte(nil), valid...)
	ne.PutUint32(badValueOff[32:36], 41)
	if _, err := validateLabels(badValueOff, 16, 1, 24); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("bad label value offset = %v, want ErrBadLayout", err)
	}

	trailing := append(append([]byte(nil), valid...), 0)
	if _, err := validateLabels(trailing, 16, 1, 24); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("trailing label bytes = %v, want ErrBadLayout", err)
	}
}

func TestLookupLabelSyntheticOverflowGuards(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("synthetic label overflow guards require 64-bit int")
	}

	maxInt := maxIntValue()
	if _, _, _, err := labelLayoutPrefix(16, maxInt/LookupLabelEntrySize+1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("huge label table layout = %v, want ErrOverflow", err)
	}

	if _, _, _, err := labelLayoutGo(maxInt-6, []struct{ Key, Value []byte }{{Key: []byte("k")}}); !errors.Is(err, ErrOverflow) {
		t.Fatalf("label layout align overflow = %v, want ErrOverflow", err)
	}
	if _, _, _, err := labelLayoutGo(maxInt-15, []struct{ Key, Value []byte }{{Key: []byte("k")}}); !errors.Is(err, ErrOverflow) {
		t.Fatalf("label layout item-size overflow = %v, want ErrOverflow", err)
	}

	maxU32Value := ^uint32(0)
	hugeBytes := int(uint64(maxU32Value) + 1)
	if _, _, _, _, _, _, ok := lookupLabelWriteLayout(16, hugeBytes, 0); ok {
		t.Fatal("huge label key write should overflow")
	}
	if _, _, _, _, _, _, ok := lookupLabelWriteLayout(16, 1, hugeBytes); ok {
		t.Fatal("huge label value write should overflow")
	}
}

func TestLookupRawResponseGuardEdges(t *testing.T) {
	if n := finishLookupResponse(make([]byte, AppsLookupRespHdr), AppsLookupRespHdr, 0, AppsLookupRespHdr, 123); n != AppsLookupRespHdr {
		t.Fatalf("empty lookup finish = %d, want %d", n, AppsLookupRespHdr)
	}

	buf := make([]byte, 64)
	ne.PutUint32(buf[AppsLookupRespHdr:AppsLookupRespHdr+4], 40)
	if n := finishLookupResponse(buf, AppsLookupRespHdr, 1, 32, 123); n != 0 {
		t.Fatalf("finish with data before first item = %d, want 0", n)
	}

	if _, err := lookupResponseRawItem(buf, AppsLookupRespHdr, 1, 1); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("raw item out of bounds = %v, want ErrOutOfBounds", err)
	}
	if _, err := EncodeAppsLookupRawResponse(nil, 1, make([]byte, AppsLookupRespHdr-1)); !errors.Is(err, ErrOverflow) {
		t.Fatalf("raw response short header = %v, want ErrOverflow", err)
	}
	if _, err := EncodeAppsLookupRawResponse([][]byte{make([]byte, AppsLookupItemHdr-1)}, 1, make([]byte, 128)); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("raw response short item = %v, want ErrBadLayout", err)
	}
	if _, err := EncodeAppsLookupRawResponse([][]byte{make([]byte, AppsLookupItemHdr)}, 1, make([]byte, 128)); !errors.Is(err, ErrBadLayout) {
		t.Fatalf("raw response invalid item = %v, want ErrBadLayout", err)
	}
}

func TestLookupThirtyTwoBitGuardCoverage(t *testing.T) {
	if strconv.IntSize != 32 {
		t.Skip("32-bit integer guard coverage requires GOARCH=386")
	}

	overMaxInt := uint32(maxIntValue()) + 1
	var dir [LookupDirEntrySize]byte
	ne.PutUint32(dir[0:4], overMaxInt)
	ne.PutUint32(dir[4:8], 0)
	if _, err := checkedWireU32Int(dir[:], 0); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("checkedWireU32Int over int range = %v, want ErrOutOfBounds", err)
	}
	if _, _, err := lookupDirEntry(dir[:], 0); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("lookupDirEntry offset over int range = %v, want ErrOutOfBounds", err)
	}
	ne.PutUint32(dir[0:4], 0)
	ne.PutUint32(dir[4:8], overMaxInt)
	if _, _, err := lookupDirEntry(dir[:], 0); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("lookupDirEntry length over int range = %v, want ErrOutOfBounds", err)
	}

	hugeLookupItems := uint32(maxIntValue()/LookupDirEntrySize) + 1
	if _, ok := lookupBuilderDataOffset(AppsLookupRespHdr, hugeLookupItems); ok {
		t.Fatal("lookupBuilderDataOffset huge item count should fail on 32-bit")
	}
	if _, ok := lookupDirOffset(AppsLookupRespHdr, hugeLookupItems); ok {
		t.Fatal("lookupDirOffset huge index should fail on 32-bit")
	}

	if _, err := BatchDirDecode(nil, overMaxInt, 0); !errors.Is(err, ErrBadItemCount) {
		t.Fatalf("BatchDirDecode over int range = %v, want ErrBadItemCount", err)
	}
	if err := BatchDirValidate(nil, overMaxInt, 0); !errors.Is(err, ErrBadItemCount) {
		t.Fatalf("BatchDirValidate over int range = %v, want ErrBadItemCount", err)
	}
	if _, err := BatchItemGet(make([]byte, LookupDirEntrySize), overMaxInt, 0); !errors.Is(err, ErrBadItemCount) {
		t.Fatalf("BatchItemGet over int range = %v, want ErrBadItemCount", err)
	}
	batchDir := make([]byte, LookupDirEntrySize)
	ne.PutUint32(batchDir[0:4], overMaxInt)
	ne.PutUint32(batchDir[4:8], 0)
	if _, err := BatchItemGet(batchDir, 1, 0); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("BatchItemGet over-int offset = %v, want ErrOutOfBounds", err)
	}
	ne.PutUint32(batchDir[0:4], 0)
	ne.PutUint32(batchDir[4:8], overMaxInt)
	if _, err := BatchItemGet(batchDir, 1, 0); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("BatchItemGet over-int length = %v, want ErrOutOfBounds", err)
	}
	if _, err := BatchDirDecode(nil, hugeLookupItems, 0); !errors.Is(err, ErrBadItemCount) {
		t.Fatalf("BatchDirDecode huge item count = %v, want ErrBadItemCount", err)
	}
	if err := BatchDirValidate(nil, hugeLookupItems, 0); !errors.Is(err, ErrBadItemCount) {
		t.Fatalf("BatchDirValidate huge item count = %v, want ErrBadItemCount", err)
	}
	if _, err := BatchItemGet([]byte{0}, hugeLookupItems, 0); !errors.Is(err, ErrBadItemCount) {
		t.Fatalf("BatchItemGet huge item count = %v, want ErrBadItemCount", err)
	}

	var batch BatchBuilder
	batch.Reset(nil, hugeLookupItems)
	if batch.dirEnd != maxIntValue() || batch.dataOffset != 0 {
		t.Fatalf("BatchBuilder huge reset = dirEnd %d dataOffset %d", batch.dirEnd, batch.dataOffset)
	}
	batch = BatchBuilder{
		buf:        make([]byte, 16),
		maxItems:   ^uint32(0),
		itemCount:  hugeLookupItems,
		dirEnd:     LookupDirEntrySize,
		dataOffset: 0,
	}
	if err := batch.Add(nil); !errors.Is(err, ErrOverflow) {
		t.Fatalf("BatchBuilder over-int item index = %v, want ErrOverflow", err)
	}

	cgroupsPayload := make([]byte, CgroupsLookupReqHdr+LookupDirEntrySize)
	cgroupsView := &CgroupsLookupRequestView{
		ItemCount:   1,
		packedStart: CgroupsLookupReqHdr + LookupDirEntrySize,
		payload:     cgroupsPayload,
	}
	ne.PutUint32(cgroupsPayload[CgroupsLookupReqHdr:CgroupsLookupReqHdr+4], 0)
	ne.PutUint32(cgroupsPayload[CgroupsLookupReqHdr+4:CgroupsLookupReqHdr+8], 0)
	if _, _, err := cgroupsView.itemBytes(0); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("cgroups itemBytes zero length = %v, want ErrOutOfBounds", err)
	}
	ne.PutUint32(cgroupsPayload[CgroupsLookupReqHdr:CgroupsLookupReqHdr+4], overMaxInt)
	ne.PutUint32(cgroupsPayload[CgroupsLookupReqHdr+4:CgroupsLookupReqHdr+8], 1)
	if _, _, err := cgroupsView.itemBytes(0); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("cgroups itemBytes over-int offset = %v, want ErrOutOfBounds", err)
	}

	if !payloadExceededSuffixFits(0, 0, nil, 0, hugeLookupItems) {
		t.Fatal("payloadExceededSuffixFits should ignore unrepresentable maxItems")
	}
	if payloadExceededSuffixFits(0, 0, make([]uint32, 2), overMaxInt, 1) {
		t.Fatal("payloadExceededSuffixFits should reject unrepresentable first index")
	}
	if _, ok := makePayloadExceededSuffixBytes(overMaxInt); ok {
		t.Fatal("makePayloadExceededSuffixBytes should reject unrepresentable item count")
	}
	if _, ok := stringReverseEncodedLen(maxIntValue()); ok {
		t.Fatal("string reverse encoded length should reject int overflow")
	}
	expectPanic(t, "apps lookup builder huge maxItems", func() {
		NewAppsLookupBuilder(nil, overMaxInt, 0)
	})
	expectPanic(t, "cgroups lookup builder huge maxItems", func() {
		NewCgroupsLookupBuilder(nil, overMaxInt, 0)
	})

	appsReq := make([]byte, AppsLookupReqHdr)
	ne.PutUint16(appsReq[0:2], 1)
	ne.PutUint32(appsReq[4:8], hugeLookupItems)
	if _, err := DecodeAppsLookupRequest(appsReq); !errors.Is(err, ErrBadItemCount) {
		t.Fatalf("DecodeAppsLookupRequest huge item count = %v, want ErrBadItemCount", err)
	}
	if _, err := (&AppsLookupRequestView{ItemCount: hugeLookupItems, payload: appsReq}).Item(0); !errors.Is(err, ErrOverflow) {
		t.Fatalf("AppsLookupRequestView.Item huge item count = %v, want ErrOverflow", err)
	}

	appsResp := make([]byte, AppsLookupRespHdr)
	ne.PutUint16(appsResp[0:2], 1)
	ne.PutUint32(appsResp[4:8], hugeLookupItems)
	if _, err := DecodeAppsLookupResponse(appsResp); !errors.Is(err, ErrBadItemCount) {
		t.Fatalf("DecodeAppsLookupResponse huge item count = %v, want ErrBadItemCount", err)
	}
	if _, err := (&AppsLookupResponseView{ItemCount: hugeLookupItems, payload: appsResp}).Item(0); !errors.Is(err, ErrOverflow) {
		t.Fatalf("AppsLookupResponseView.Item huge item count = %v, want ErrOverflow", err)
	}

	cgroupsReq := make([]byte, CgroupsLookupReqHdr)
	ne.PutUint16(cgroupsReq[0:2], 1)
	ne.PutUint32(cgroupsReq[4:8], hugeLookupItems)
	if _, err := DecodeCgroupsLookupRequest(cgroupsReq); !errors.Is(err, ErrBadItemCount) {
		t.Fatalf("DecodeCgroupsLookupRequest huge item count = %v, want ErrBadItemCount", err)
	}
	if _, err := (&CgroupsLookupRequestView{ItemCount: hugeLookupItems, payload: cgroupsReq}).Item(0); !errors.Is(err, ErrOverflow) {
		t.Fatalf("CgroupsLookupRequestView.Item huge item count = %v, want ErrOverflow", err)
	}

	cgroupsResp := make([]byte, CgroupsLookupRespHdr)
	ne.PutUint16(cgroupsResp[0:2], 1)
	ne.PutUint32(cgroupsResp[4:8], hugeLookupItems)
	if _, err := DecodeCgroupsLookupResponse(cgroupsResp); !errors.Is(err, ErrBadItemCount) {
		t.Fatalf("DecodeCgroupsLookupResponse huge item count = %v, want ErrBadItemCount", err)
	}
	if _, err := (&CgroupsLookupResponseView{ItemCount: hugeLookupItems, payload: cgroupsResp}).Item(0); !errors.Is(err, ErrOverflow) {
		t.Fatalf("CgroupsLookupResponseView.Item huge item count = %v, want ErrOverflow", err)
	}

	cgroupsSnapshotResp := make([]byte, cgroupsRespHdr)
	ne.PutUint16(cgroupsSnapshotResp[0:2], 1)
	ne.PutUint32(cgroupsSnapshotResp[4:8], uint32(maxIntValue()/cgroupsDirEntry)+1)
	if _, err := DecodeCgroupsResponse(cgroupsSnapshotResp); !errors.Is(err, ErrBadItemCount) {
		t.Fatalf("DecodeCgroupsResponse huge item count = %v, want ErrBadItemCount", err)
	}
	if _, err := (&CgroupsResponseView{ItemCount: uint32(maxIntValue()/cgroupsDirEntry) + 1, payload: cgroupsSnapshotResp}).Item(0); !errors.Is(err, ErrBadItemCount) {
		t.Fatalf("CgroupsResponseView.Item huge item count = %v, want ErrBadItemCount", err)
	}
}
