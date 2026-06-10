package protocol

import "testing"

func labels(items ...struct{ Key, Value []byte }) []struct{ Key, Value []byte } {
	return items
}

func TestCgroupsLookupRoundTrip(t *testing.T) {
	var req [256]byte
	n, err := EncodeCgroupsLookupRequest([][]byte{
		[]byte("/sys/fs/cgroup/a"),
		[]byte("/system.slice/docker-abc.scope"),
	}, req[:])
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	reqView, err := DecodeCgroupsLookupRequest(req[:n])
	if err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if reqView.ItemCount != 2 {
		t.Fatalf("item count = %d, want 2", reqView.ItemCount)
	}
	item0, err := reqView.Item(0)
	if err != nil || item0.String() != "/sys/fs/cgroup/a" {
		t.Fatalf("item 0 = %q, err=%v", item0.String(), err)
	}

	var resp [1024]byte
	builder := NewCgroupsLookupBuilder(resp[:], 2, 123)
	if err := builder.Add(
		CgroupLookupKnown,
		OrchestratorK8s,
		[]byte("/sys/fs/cgroup/a"),
		[]byte("pod-a"),
		labels(struct{ Key, Value []byte }{[]byte("namespace"), []byte("default")}),
	); err != nil {
		t.Fatalf("add known: %v", err)
	}
	if err := builder.Add(
		CgroupLookupUnknownPermanent,
		0,
		[]byte("/system.slice/docker-abc.scope"),
		nil,
		nil,
	); err != nil {
		t.Fatalf("add unknown: %v", err)
	}
	total := builder.Finish()
	view, err := DecodeCgroupsLookupResponse(resp[:total])
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if view.ItemCount != 2 || view.Generation != 123 {
		t.Fatalf("header = count %d generation %d", view.ItemCount, view.Generation)
	}
	got, err := view.Item(0)
	if err != nil {
		t.Fatalf("response item 0: %v", err)
	}
	if got.Status != CgroupLookupKnown || got.Orchestrator != OrchestratorK8s ||
		got.Path.String() != "/sys/fs/cgroup/a" || got.Name.String() != "pod-a" ||
		got.LabelCount != 1 {
		t.Fatalf("bad known item: %+v", got)
	}
	label, err := got.Label(0)
	if err != nil {
		t.Fatalf("label 0: %v", err)
	}
	if label.Key.String() != "namespace" || label.Value.String() != "default" {
		t.Fatalf("bad label: %q=%q", label.Key.String(), label.Value.String())
	}
	got, err = view.Item(1)
	if err != nil {
		t.Fatalf("response item 1: %v", err)
	}
	if got.Status != CgroupLookupUnknownPermanent || got.Name.Len() != 0 {
		t.Fatalf("bad unknown item: %+v", got)
	}
}

func TestAppsLookupRoundTrip(t *testing.T) {
	var req [128]byte
	n, err := EncodeAppsLookupRequest([]uint32{0, 1234, 9999}, req[:])
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	reqView, err := DecodeAppsLookupRequest(req[:n])
	if err != nil {
		t.Fatalf("decode request: %v", err)
	}
	pid, err := reqView.Item(0)
	if err != nil || pid != 0 {
		t.Fatalf("item 0 pid = %d, err=%v", pid, err)
	}

	var resp [2048]byte
	builder := NewAppsLookupBuilder(resp[:], 3, 77)
	if err := builder.Add(
		PidLookupKnown,
		AppsCgroupKnown,
		OrchestratorDocker,
		1234,
		1,
		1000,
		^uint64(0),
		[]byte("nginx"),
		[]byte("/docker/abc"),
		[]byte("container-a"),
		labels(struct{ Key, Value []byte }{[]byte("image"), []byte("nginx:latest")}),
	); err != nil {
		t.Fatalf("add known: %v", err)
	}
	if err := builder.Add(
		PidLookupKnown,
		AppsCgroupHostRoot,
		0,
		0,
		0,
		0,
		0,
		[]byte("swapper"),
		nil,
		nil,
		nil,
	); err != nil {
		t.Fatalf("add host root: %v", err)
	}
	if err := builder.Add(
		PidLookupUnknown,
		AppsCgroupKnown,
		0,
		9999,
		0,
		NipcUIDUnset,
		0,
		nil,
		nil,
		nil,
		nil,
	); err != nil {
		t.Fatalf("add unknown: %v", err)
	}
	total := builder.Finish()
	view, err := DecodeAppsLookupResponse(resp[:total])
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if view.ItemCount != 3 || view.Generation != 77 {
		t.Fatalf("header = count %d generation %d", view.ItemCount, view.Generation)
	}
	item0, err := view.Item(0)
	if err != nil {
		t.Fatalf("item 0: %v", err)
	}
	if item0.Pid != 1234 || item0.Status != PidLookupKnown ||
		item0.CgroupStatus != AppsCgroupKnown ||
		item0.Comm.String() != "nginx" ||
		item0.CgroupPath.String() != "/docker/abc" ||
		item0.Starttime != ^uint64(0) {
		t.Fatalf("bad known item: %+v", item0)
	}
	item1, err := view.Item(1)
	if err != nil {
		t.Fatalf("item 1: %v", err)
	}
	if item1.Pid != 0 || item1.CgroupStatus != AppsCgroupHostRoot || item1.CgroupPath.Len() != 0 {
		t.Fatalf("bad host-root item: %+v", item1)
	}
	item2, err := view.Item(2)
	if err != nil {
		t.Fatalf("item 2: %v", err)
	}
	if item2.Pid != 9999 || item2.Status != PidLookupUnknown || item2.Uid != NipcUIDUnset {
		t.Fatalf("bad unknown item: %+v", item2)
	}
}

func TestLookupValidationEdges(t *testing.T) {
	var req [128]byte
	if _, err := EncodeCgroupsLookupRequest([][]byte{[]byte("bad\x00path")}, req[:]); err != ErrBadLayout {
		t.Fatalf("interior NUL request error = %v, want ErrBadLayout", err)
	}

	var resp [256]byte
	builder := NewAppsLookupBuilder(resp[:], 1, 0)
	if err := builder.Add(
		PidLookupKnown,
		AppsCgroupHostRoot,
		0,
		1,
		0,
		0,
		1,
		[]byte("1234567890123456"),
		nil,
		nil,
		nil,
	); err != ErrBadLayout {
		t.Fatalf("comm len 16 error = %v, want ErrBadLayout", err)
	}

	cg := NewCgroupsLookupBuilder(resp[:], 1, 0)
	if err := cg.Add(CgroupLookupKnown, 99, []byte("/x"), nil, nil); err != nil {
		t.Fatalf("unknown orchestrator should be accepted: %v", err)
	}
	total := cg.Finish()
	itemStart := CgroupsLookupRespHdr + LookupDirEntrySize + int(ne.Uint32(resp[CgroupsLookupRespHdr:CgroupsLookupRespHdr+4]))
	ne.PutUint16(resp[itemStart+2:itemStart+4], 99)
	if _, err := DecodeCgroupsLookupResponse(resp[:total]); err != ErrBadLayout {
		t.Fatalf("bad status error = %v, want ErrBadLayout", err)
	}
}

func TestLookupDispatchRejectsShortResponseBuffer(t *testing.T) {
	var req [128]byte
	reqLen, err := EncodeCgroupsLookupRequest([][]byte{[]byte("/x")}, req[:])
	if err != nil {
		t.Fatalf("encode cgroups request: %v", err)
	}
	shortCgroups := make([]byte, CgroupsLookupRespHdr+LookupDirEntrySize-1)
	n, err := DispatchCgroupsLookup(req[:reqLen], shortCgroups, func(*CgroupsLookupRequestView, *CgroupsLookupBuilder) bool {
		t.Fatal("handler should not run with undersized response buffer")
		return false
	})
	if err != ErrOverflow || n != 0 {
		t.Fatalf("cgroups dispatch = n %d err %v, want 0 ErrOverflow", n, err)
	}

	reqLen, err = EncodeAppsLookupRequest([]uint32{1234}, req[:])
	if err != nil {
		t.Fatalf("encode apps request: %v", err)
	}
	shortApps := make([]byte, AppsLookupRespHdr+LookupDirEntrySize-1)
	n, err = DispatchAppsLookup(req[:reqLen], shortApps, func(*AppsLookupRequestView, *AppsLookupBuilder) bool {
		t.Fatal("handler should not run with undersized response buffer")
		return false
	})
	if err != ErrOverflow || n != 0 {
		t.Fatalf("apps dispatch = n %d err %v, want 0 ErrOverflow", n, err)
	}
}

func TestLookupDecodeRejectsMaxUint32DirectoryOffset(t *testing.T) {
	var resp [512]byte
	cg := NewCgroupsLookupBuilder(resp[:], 1, 0)
	if err := cg.Add(CgroupLookupKnown, OrchestratorDocker, []byte("/x"), nil, nil); err != nil {
		t.Fatalf("add cgroups item: %v", err)
	}
	total := cg.Finish()
	ne.PutUint32(resp[CgroupsLookupRespHdr:CgroupsLookupRespHdr+4], ^uint32(0)-7)
	if _, err := DecodeCgroupsLookupResponse(resp[:total]); err != ErrOutOfBounds {
		t.Fatalf("cgroups response with max offset error = %v, want ErrOutOfBounds", err)
	}

	apps := NewAppsLookupBuilder(resp[:], 1, 0)
	if err := apps.Add(
		PidLookupKnown,
		AppsCgroupHostRoot,
		0,
		1234,
		1,
		1000,
		42,
		[]byte("nginx"),
		nil,
		nil,
		nil,
	); err != nil {
		t.Fatalf("add apps item: %v", err)
	}
	total = apps.Finish()
	ne.PutUint32(resp[AppsLookupRespHdr:AppsLookupRespHdr+4], ^uint32(0)-7)
	if _, err := DecodeAppsLookupResponse(resp[:total]); err != ErrOutOfBounds {
		t.Fatalf("apps response with max offset error = %v, want ErrOutOfBounds", err)
	}
}

func TestLookupLabelLayoutOverflow(t *testing.T) {
	sample := labels(struct{ Key, Value []byte }{[]byte("k"), []byte("v")})
	maxInt := int(^uint(0) >> 1)

	if _, _, _, err := labelLayoutGo(maxInt-3, sample); err != ErrOverflow {
		t.Fatalf("align overflow error = %v, want ErrOverflow", err)
	}
	if _, _, _, err := labelLayoutGo(maxInt-15, sample); err != ErrOverflow {
		t.Fatalf("label table overflow error = %v, want ErrOverflow", err)
	}
}

func expectPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s did not panic", name)
		}
	}()
	fn()
}

func cgroupsLookupItemStart(t *testing.T, buf []byte) int {
	t.Helper()
	return CgroupsLookupRespHdr + LookupDirEntrySize +
		int(ne.Uint32(buf[CgroupsLookupRespHdr:CgroupsLookupRespHdr+4]))
}

func appsLookupItemStart(t *testing.T, buf []byte) int {
	t.Helper()
	return AppsLookupRespHdr + LookupDirEntrySize +
		int(ne.Uint32(buf[AppsLookupRespHdr:AppsLookupRespHdr+4]))
}

func validCgroupsLookupItemBytes(t *testing.T) []byte {
	t.Helper()
	var resp [512]byte
	builder := NewCgroupsLookupBuilder(resp[:], 1, 0)
	if err := builder.Add(
		CgroupLookupKnown,
		OrchestratorDocker,
		[]byte("/x"),
		[]byte("name"),
		labels(struct{ Key, Value []byte }{[]byte("k"), []byte("v")}),
	); err != nil {
		t.Fatalf("add cgroups item: %v", err)
	}
	total := builder.Finish()
	start := cgroupsLookupItemStart(t, resp[:])
	length := int(ne.Uint32(resp[CgroupsLookupRespHdr+4 : CgroupsLookupRespHdr+8]))
	return append([]byte(nil), resp[start:start+length]...)[:min(length, total-start)]
}

func validAppsLookupItemBytes(t *testing.T) []byte {
	t.Helper()
	var resp [1024]byte
	builder := NewAppsLookupBuilder(resp[:], 1, 0)
	if err := builder.Add(
		PidLookupKnown,
		AppsCgroupKnown,
		OrchestratorDocker,
		1234, 1, 1000, 42,
		[]byte("nginx"),
		[]byte("/docker/abc"),
		[]byte("container-a"),
		labels(struct{ Key, Value []byte }{[]byte("k"), []byte("v")}),
	); err != nil {
		t.Fatalf("add apps item: %v", err)
	}
	total := builder.Finish()
	start := appsLookupItemStart(t, resp[:])
	length := int(ne.Uint32(resp[AppsLookupRespHdr+4 : AppsLookupRespHdr+8]))
	return append([]byte(nil), resp[start:start+length]...)[:min(length, total-start)]
}

func TestLookupRequestValidationCoverage(t *testing.T) {
	var req [128]byte

	n, err := EncodeCgroupsLookupRequest(nil, req[:])
	if err != nil {
		t.Fatalf("encode empty cgroups request: %v", err)
	}
	cgView, err := DecodeCgroupsLookupRequest(req[:n])
	if err != nil || cgView.ItemCount != 0 {
		t.Fatalf("decode empty cgroups request = count %d err %v", cgView.ItemCount, err)
	}
	if _, err := cgView.Item(0); err != ErrOutOfBounds {
		t.Fatalf("empty cgroups item error = %v, want ErrOutOfBounds", err)
	}
	if _, err := EncodeCgroupsLookupRequest([][]byte{[]byte("/x")}, req[:CgroupsLookupReqHdr]); err != ErrOverflow {
		t.Fatalf("short cgroups request buffer error = %v, want ErrOverflow", err)
	}
	if _, err := EncodeCgroupsLookupRequest([][]byte{[]byte("/xy")}, make([]byte, CgroupsLookupReqHdr+LookupDirEntrySize+1)); err != ErrOverflow {
		t.Fatalf("packed cgroups request overflow error = %v, want ErrOverflow", err)
	}

	n, err = EncodeCgroupsLookupRequest([][]byte{[]byte("/xy")}, req[:])
	if err != nil {
		t.Fatalf("encode cgroups request: %v", err)
	}
	if _, err := DecodeCgroupsLookupRequest(req[:CgroupsLookupReqHdr]); err != ErrTruncated {
		t.Fatalf("cgroups request truncated dir error = %v, want ErrTruncated", err)
	}
	cgManualReq := make([]byte, CgroupsLookupReqHdr+LookupDirEntrySize)
	ne.PutUint32(cgManualReq[CgroupsLookupReqHdr+4:CgroupsLookupReqHdr+8], 8)
	if _, err := (&CgroupsLookupRequestView{ItemCount: 1, payload: cgManualReq}).Item(0); err != ErrOutOfBounds {
		t.Fatalf("manual cgroups request item error = %v, want ErrOutOfBounds", err)
	}
	for _, tc := range []struct {
		name string
		edit func([]byte)
		want error
	}{
		{
			name: "bad layout",
			edit: func(b []byte) {
				ne.PutUint16(b[0:2], 2)
			},
			want: ErrBadLayout,
		},
		{
			name: "bad flags",
			edit: func(b []byte) {
				ne.PutUint16(b[2:4], 1)
			},
			want: ErrBadLayout,
		},
		{
			name: "bad reserved",
			edit: func(b []byte) {
				ne.PutUint32(b[8:12], 1)
			},
			want: ErrBadLayout,
		},
		{
			name: "bad alignment",
			edit: func(b []byte) {
				ne.PutUint32(b[CgroupsLookupReqHdr:CgroupsLookupReqHdr+4], 1)
			},
			want: ErrBadAlignment,
		},
		{
			name: "too short item",
			edit: func(b []byte) {
				ne.PutUint32(b[CgroupsLookupReqHdr+4:CgroupsLookupReqHdr+8], 1)
			},
			want: ErrBadLayout,
		},
		{
			name: "missing nul",
			edit: func(b []byte) {
				dirEnd := CgroupsLookupReqHdr + LookupDirEntrySize
				b[dirEnd+3] = '!'
			},
			want: ErrMissingNul,
		},
		{
			name: "interior nul",
			edit: func(b []byte) {
				dirEnd := CgroupsLookupReqHdr + LookupDirEntrySize
				b[dirEnd+1] = 0
			},
			want: ErrBadLayout,
		},
	} {
		bad := append([]byte(nil), req[:n]...)
		tc.edit(bad)
		if _, err := DecodeCgroupsLookupRequest(bad); err != tc.want {
			t.Fatalf("decode cgroups request %s error = %v, want %v", tc.name, err, tc.want)
		}
	}

	n, err = EncodeAppsLookupRequest(nil, req[:])
	if err != nil {
		t.Fatalf("encode empty apps request: %v", err)
	}
	appsView, err := DecodeAppsLookupRequest(req[:n])
	if err != nil || appsView.ItemCount != 0 {
		t.Fatalf("decode empty apps request = count %d err %v", appsView.ItemCount, err)
	}
	if _, err := appsView.Item(0); err != ErrOutOfBounds {
		t.Fatalf("empty apps item error = %v, want ErrOutOfBounds", err)
	}
	if _, err := EncodeAppsLookupRequest([]uint32{1234}, req[:AppsLookupReqHdr]); err != ErrOverflow {
		t.Fatalf("short apps request buffer error = %v, want ErrOverflow", err)
	}
	if _, err := EncodeAppsLookupRequest([]uint32{1234}, make([]byte, AppsLookupReqHdr+LookupDirEntrySize+AppsLookupKeySize-1)); err != ErrOverflow {
		t.Fatalf("packed apps request overflow error = %v, want ErrOverflow", err)
	}

	n, err = EncodeAppsLookupRequest([]uint32{1234}, req[:])
	if err != nil {
		t.Fatalf("encode apps request: %v", err)
	}
	if _, err := DecodeAppsLookupRequest(req[:AppsLookupReqHdr]); err != ErrTruncated {
		t.Fatalf("apps request truncated dir error = %v, want ErrTruncated", err)
	}
	appsManualReq := make([]byte, AppsLookupReqHdr+LookupDirEntrySize)
	ne.PutUint32(appsManualReq[AppsLookupReqHdr+4:AppsLookupReqHdr+8], AppsLookupKeySize)
	if _, err := (&AppsLookupRequestView{ItemCount: 1, payload: appsManualReq}).Item(0); err != ErrOutOfBounds {
		t.Fatalf("manual apps request item error = %v, want ErrOutOfBounds", err)
	}
	for _, tc := range []struct {
		name string
		edit func([]byte)
		want error
	}{
		{
			name: "bad layout",
			edit: func(b []byte) {
				ne.PutUint16(b[0:2], 2)
			},
			want: ErrBadLayout,
		},
		{
			name: "bad item length",
			edit: func(b []byte) {
				ne.PutUint32(b[AppsLookupReqHdr+4:AppsLookupReqHdr+8], AppsLookupKeySize-1)
			},
			want: ErrBadLayout,
		},
		{
			name: "bad key reserved",
			edit: func(b []byte) {
				dirEnd := AppsLookupReqHdr + LookupDirEntrySize
				ne.PutUint32(b[dirEnd+4:dirEnd+8], 1)
			},
			want: ErrBadLayout,
		},
	} {
		bad := append([]byte(nil), req[:n]...)
		tc.edit(bad)
		if _, err := DecodeAppsLookupRequest(bad); err != tc.want {
			t.Fatalf("decode apps request %s error = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestCgroupsLookupBuilderGuardCoverage(t *testing.T) {
	expectPanic(t, "small cgroups builder", func() {
		_ = NewCgroupsLookupBuilder(make([]byte, CgroupsLookupRespHdr+LookupDirEntrySize-1), 1, 0)
	})

	var empty [CgroupsLookupRespHdr]byte
	emptyBuilder := NewCgroupsLookupBuilder(empty[:], 0, 1)
	emptyBuilder.SetGeneration(2)
	if total := emptyBuilder.Finish(); total != CgroupsLookupRespHdr {
		t.Fatalf("empty cgroups finish = %d", total)
	}
	emptyView, err := DecodeCgroupsLookupResponse(empty[:])
	if err != nil || emptyView.Generation != 2 || emptyView.ItemCount != 0 {
		t.Fatalf("empty cgroups decode = generation %d count %d err %v", emptyView.Generation, emptyView.ItemCount, err)
	}

	var resp [512]byte
	builder := NewCgroupsLookupBuilder(resp[:], 1, 10)
	builder.SetGeneration(11)
	if builder.ItemCount() != 0 || builder.Error() != nil {
		t.Fatalf("fresh cgroups builder count/error = %d/%v", builder.ItemCount(), builder.Error())
	}
	err = builder.Add(
		CgroupLookupKnown,
		OrchestratorK8s,
		[]byte("/x"),
		[]byte("pod"),
		labels(struct{ Key, Value []byte }{[]byte("namespace"), []byte("default")}),
	)
	if err != nil {
		t.Fatalf("add cgroups known: %v", err)
	}
	if builder.ItemCount() != 1 || builder.Error() != nil {
		t.Fatalf("used cgroups builder count/error = %d/%v", builder.ItemCount(), builder.Error())
	}
	if err := builder.Add(CgroupLookupKnown, 0, []byte("/overflow"), nil, nil); err != ErrOverflow {
		t.Fatalf("cgroups overflow error = %v, want ErrOverflow", err)
	}
	if builder.Error() != ErrOverflow {
		t.Fatalf("cgroups builder error = %v, want ErrOverflow", builder.Error())
	}
	total := builder.Finish()
	view, err := DecodeCgroupsLookupResponse(resp[:total])
	if err != nil || view.Generation != 11 {
		t.Fatalf("decode cgroups builder response = generation %d err %v", view.Generation, err)
	}
	item, err := view.Item(0)
	if err != nil {
		t.Fatalf("cgroups response item: %v", err)
	}
	if _, err := item.Label(1); err != ErrOutOfBounds {
		t.Fatalf("cgroups label out-of-bounds error = %v, want ErrOutOfBounds", err)
	}
	if _, err := view.Item(1); err != ErrOutOfBounds {
		t.Fatalf("cgroups item out-of-bounds error = %v, want ErrOutOfBounds", err)
	}

	for _, tc := range []struct {
		name string
		add  func(*CgroupsLookupBuilder) error
		want error
	}{
		{
			name: "bad status",
			add: func(b *CgroupsLookupBuilder) error {
				return b.Add(99, 0, []byte("/x"), nil, nil)
			},
			want: ErrBadLayout,
		},
		{
			name: "empty path",
			add: func(b *CgroupsLookupBuilder) error {
				return b.Add(CgroupLookupKnown, 0, nil, nil, nil)
			},
			want: ErrBadLayout,
		},
		{
			name: "bad name",
			add: func(b *CgroupsLookupBuilder) error {
				return b.Add(CgroupLookupKnown, 0, []byte("/x"), []byte("bad\x00name"), nil)
			},
			want: ErrBadLayout,
		},
		{
			name: "unknown with name",
			add: func(b *CgroupsLookupBuilder) error {
				return b.Add(CgroupLookupUnknownRetryLater, 0, []byte("/x"), []byte("name"), nil)
			},
			want: ErrBadLayout,
		},
		{
			name: "bad label",
			add: func(b *CgroupsLookupBuilder) error {
				return b.Add(CgroupLookupKnown, 0, []byte("/x"), nil,
					labels(struct{ Key, Value []byte }{[]byte{}, []byte("v")}))
			},
			want: ErrBadLayout,
		},
	} {
		b := NewCgroupsLookupBuilder(resp[:], 1, 0)
		if err := tc.add(b); err != tc.want {
			t.Fatalf("cgroups builder %s error = %v, want %v", tc.name, err, tc.want)
		}
	}
	tooManyLabels := make([]struct{ Key, Value []byte }, int(^uint16(0))+1)
	if err := NewCgroupsLookupBuilder(resp[:], 1, 0).Add(CgroupLookupKnown, 0, []byte("/x"), nil, tooManyLabels); err != ErrOverflow {
		t.Fatalf("cgroups too-many-labels error = %v, want ErrOverflow", err)
	}

	small := make([]byte, CgroupsLookupRespHdr+LookupDirEntrySize)
	smallBuilder := NewCgroupsLookupBuilder(small, 1, 0)
	if err := smallBuilder.Add(CgroupLookupKnown, 0, []byte("/x"), nil, nil); err != ErrOverflow {
		t.Fatalf("small cgroups builder add error = %v, want ErrOverflow", err)
	}
	negativeOffsetBuilder := &CgroupsLookupBuilder{buf: make([]byte, 512), maxItems: 1, dataOffset: -1}
	if err := negativeOffsetBuilder.Add(CgroupLookupKnown, 0, []byte("/x"), nil, nil); err != ErrOverflow {
		t.Fatalf("negative-offset cgroups builder add error = %v, want ErrOverflow", err)
	}
	overflowOffsetBuilder := &CgroupsLookupBuilder{buf: make([]byte, 512), maxItems: 1, dataOffset: maxIntValue() - 32}
	if err := overflowOffsetBuilder.Add(CgroupLookupKnown, 0, []byte("/x"), nil, nil); err != ErrOverflow {
		t.Fatalf("overflow-offset cgroups builder add error = %v, want ErrOverflow", err)
	}
}

func TestAppsLookupBuilderGuardCoverage(t *testing.T) {
	expectPanic(t, "small apps builder", func() {
		_ = NewAppsLookupBuilder(make([]byte, AppsLookupRespHdr+LookupDirEntrySize-1), 1, 0)
	})

	var empty [AppsLookupRespHdr]byte
	emptyBuilder := NewAppsLookupBuilder(empty[:], 0, 1)
	emptyBuilder.SetGeneration(2)
	if total := emptyBuilder.Finish(); total != AppsLookupRespHdr {
		t.Fatalf("empty apps finish = %d", total)
	}
	emptyView, err := DecodeAppsLookupResponse(empty[:])
	if err != nil || emptyView.Generation != 2 || emptyView.ItemCount != 0 {
		t.Fatalf("empty apps decode = generation %d count %d err %v", emptyView.Generation, emptyView.ItemCount, err)
	}

	var resp [1024]byte
	builder := NewAppsLookupBuilder(resp[:], 4, 20)
	builder.SetGeneration(21)
	if builder.ItemCount() != 0 || builder.Error() != nil {
		t.Fatalf("fresh apps builder count/error = %d/%v", builder.ItemCount(), builder.Error())
	}
	if err := builder.Add(
		PidLookupKnown,
		AppsCgroupKnown,
		OrchestratorDocker,
		1234, 1, 1000, 42,
		[]byte("nginx"),
		[]byte("/docker/abc"),
		[]byte("container-a"),
		labels(struct{ Key, Value []byte }{[]byte("image"), []byte("nginx")}),
	); err != nil {
		t.Fatalf("add apps known: %v", err)
	}
	if err := builder.Add(
		PidLookupKnown,
		AppsCgroupUnknownRetryLater,
		0,
		1235, 1, 1000, 43,
		[]byte("worker"),
		nil,
		nil,
		nil,
	); err != nil {
		t.Fatalf("add apps retry: %v", err)
	}
	if err := builder.Add(
		PidLookupKnown,
		AppsCgroupUnknownPermanent,
		0,
		1236, 1, 1000, 44,
		[]byte("worker2"),
		[]byte("/gone"),
		nil,
		nil,
	); err != nil {
		t.Fatalf("add apps permanent: %v", err)
	}
	if err := builder.Add(
		PidLookupUnknown,
		0,
		0,
		9999, 0, NipcUIDUnset, 0,
		nil, nil, nil, nil,
	); err != nil {
		t.Fatalf("add apps unknown: %v", err)
	}
	if builder.ItemCount() != 4 || builder.Error() != nil {
		t.Fatalf("used apps builder count/error = %d/%v", builder.ItemCount(), builder.Error())
	}
	total := builder.Finish()
	view, err := DecodeAppsLookupResponse(resp[:total])
	if err != nil || view.Generation != 21 {
		t.Fatalf("decode apps builder response = generation %d err %v", view.Generation, err)
	}
	item, err := view.Item(0)
	if err != nil {
		t.Fatalf("apps response item: %v", err)
	}
	if _, err := item.Label(1); err != ErrOutOfBounds {
		t.Fatalf("apps label out-of-bounds error = %v, want ErrOutOfBounds", err)
	}
	if _, err := view.Item(4); err != ErrOutOfBounds {
		t.Fatalf("apps item out-of-bounds error = %v, want ErrOutOfBounds", err)
	}
	item, err = view.Item(1)
	if err != nil || item.CgroupStatus != AppsCgroupUnknownRetryLater {
		t.Fatalf("apps retry item = %+v err %v", item, err)
	}
	if len(item.CgroupPath.Bytes()) != 0 {
		t.Fatalf("apps retry cgroup path = %q, want empty", item.CgroupPath.Bytes())
	}
	item, err = view.Item(2)
	if err != nil || item.CgroupStatus != AppsCgroupUnknownPermanent {
		t.Fatalf("apps permanent item = %+v err %v", item, err)
	}
	item, err = view.Item(3)
	if err != nil || item.Status != PidLookupUnknown {
		t.Fatalf("apps unknown item = %+v err %v", item, err)
	}
	if err := builder.Add(PidLookupUnknown, 0, 0, 10000, 0, NipcUIDUnset, 0, nil, nil, nil, nil); err != ErrOverflow {
		t.Fatalf("apps overflow error = %v, want ErrOverflow", err)
	}
	if builder.Error() != ErrOverflow {
		t.Fatalf("apps builder error = %v, want ErrOverflow", builder.Error())
	}

	for _, tc := range []struct {
		name string
		add  func(*AppsLookupBuilder) error
		want error
	}{
		{
			name: "bad status",
			add: func(b *AppsLookupBuilder) error {
				return b.Add(99, 0, 0, 1, 0, 0, 0, []byte("x"), nil, nil, nil)
			},
			want: ErrBadLayout,
		},
		{
			name: "bad cgroup status",
			add: func(b *AppsLookupBuilder) error {
				return b.Add(PidLookupKnown, 99, 0, 1, 0, 0, 0, []byte("x"), nil, nil, nil)
			},
			want: ErrBadLayout,
		},
		{
			name: "bad comm",
			add: func(b *AppsLookupBuilder) error {
				return b.Add(PidLookupKnown, AppsCgroupHostRoot, 0, 1, 0, 0, 0, []byte("bad\x00comm"), nil, nil, nil)
			},
			want: ErrBadLayout,
		},
		{
			name: "unknown with data",
			add: func(b *AppsLookupBuilder) error {
				return b.Add(PidLookupUnknown, 0, 0, 1, 2, NipcUIDUnset, 0, nil, nil, nil, nil)
			},
			want: ErrBadLayout,
		},
		{
			name: "known missing path",
			add: func(b *AppsLookupBuilder) error {
				return b.Add(PidLookupKnown, AppsCgroupKnown, 0, 1, 0, 0, 0, []byte("x"), nil, nil, nil)
			},
			want: ErrBadLayout,
		},
		{
			name: "retry with labels",
			add: func(b *AppsLookupBuilder) error {
				return b.Add(PidLookupKnown, AppsCgroupUnknownRetryLater, 0, 1, 0, 0, 0,
					[]byte("x"), []byte("/x"), nil,
					labels(struct{ Key, Value []byte }{[]byte("k"), []byte("v")}))
			},
			want: ErrBadLayout,
		},
		{
			name: "permanent missing path",
			add: func(b *AppsLookupBuilder) error {
				return b.Add(PidLookupKnown, AppsCgroupUnknownPermanent, 0, 1, 0, 0, 0,
					[]byte("x"), nil, nil, nil)
			},
			want: ErrBadLayout,
		},
		{
			name: "host root with path",
			add: func(b *AppsLookupBuilder) error {
				return b.Add(PidLookupKnown, AppsCgroupHostRoot, 0, 1, 0, 0, 0, []byte("x"), []byte("/x"), nil, nil)
			},
			want: ErrBadLayout,
		},
		{
			name: "bad label",
			add: func(b *AppsLookupBuilder) error {
				return b.Add(PidLookupKnown, AppsCgroupKnown, 0, 1, 0, 0, 0,
					[]byte("x"), []byte("/x"), nil,
					labels(struct{ Key, Value []byte }{[]byte("bad\x00key"), []byte("v")}))
			},
			want: ErrBadLayout,
		},
	} {
		b := NewAppsLookupBuilder(resp[:], 1, 0)
		if err := tc.add(b); err != tc.want {
			t.Fatalf("apps builder %s error = %v, want %v", tc.name, err, tc.want)
		}
	}
	tooManyLabels := make([]struct{ Key, Value []byte }, int(^uint16(0))+1)
	if err := NewAppsLookupBuilder(resp[:], 1, 0).Add(PidLookupKnown, AppsCgroupKnown, 0, 1, 0, 0, 0, []byte("x"), []byte("/x"), nil, tooManyLabels); err != ErrOverflow {
		t.Fatalf("apps too-many-labels error = %v, want ErrOverflow", err)
	}

	small := make([]byte, AppsLookupRespHdr+LookupDirEntrySize)
	smallBuilder := NewAppsLookupBuilder(small, 1, 0)
	if err := smallBuilder.Add(PidLookupKnown, AppsCgroupHostRoot, 0, 1, 0, 0, 0, []byte("x"), nil, nil, nil); err != ErrOverflow {
		t.Fatalf("small apps builder add error = %v, want ErrOverflow", err)
	}
	negativeOffsetBuilder := &AppsLookupBuilder{buf: make([]byte, 512), maxItems: 1, dataOffset: -1}
	if err := negativeOffsetBuilder.Add(PidLookupKnown, AppsCgroupHostRoot, 0, 1, 0, 0, 0, []byte("x"), nil, nil, nil); err != ErrOverflow {
		t.Fatalf("negative-offset apps builder add error = %v, want ErrOverflow", err)
	}
	overflowOffsetBuilder := &AppsLookupBuilder{buf: make([]byte, 512), maxItems: 1, dataOffset: maxIntValue() - 32}
	if err := overflowOffsetBuilder.Add(PidLookupKnown, AppsCgroupHostRoot, 0, 1, 0, 0, 0, []byte("x"), nil, nil, nil); err != ErrOverflow {
		t.Fatalf("overflow-offset apps builder add error = %v, want ErrOverflow", err)
	}
}

func TestAppsLookupUnknownItemCanonicalLayout(t *testing.T) {
	var resp [128]byte
	builder := NewAppsLookupBuilder(resp[:], 1, 7)
	if err := builder.Add(PidLookupUnknown, AppsCgroupKnown, 0, 4321, 0, NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
		t.Fatalf("add unknown apps item: %v", err)
	}
	total := builder.Finish()
	itemStart := appsLookupItemStart(t, resp[:total])
	item := resp[itemStart:total]

	if got := len(item); got != appsLookupUnknownItemSize {
		t.Fatalf("unknown item size = %d, want %d", got, appsLookupUnknownItemSize)
	}
	if got := ne.Uint32(resp[AppsLookupRespHdr+4 : AppsLookupRespHdr+8]); got != appsLookupUnknownItemSize {
		t.Fatalf("directory item length = %d, want %d", got, appsLookupUnknownItemSize)
	}
	if got := ne.Uint32(item[32:36]); got != AppsLookupItemHdr {
		t.Fatalf("comm offset = %d, want %d", got, AppsLookupItemHdr)
	}
	if got := ne.Uint32(item[40:44]); got != AppsLookupItemHdr+1 {
		t.Fatalf("path offset = %d, want %d", got, AppsLookupItemHdr+1)
	}
	if got := ne.Uint32(item[48:52]); got != AppsLookupItemHdr+2 {
		t.Fatalf("name offset = %d, want %d", got, AppsLookupItemHdr+2)
	}
	if item[AppsLookupItemHdr] != 0 || item[AppsLookupItemHdr+1] != 0 || item[AppsLookupItemHdr+2] != 0 {
		t.Fatalf("unknown item NUL bytes = %v, want all zero", item[AppsLookupItemHdr:AppsLookupItemHdr+3])
	}

	view, err := DecodeAppsLookupResponse(resp[:total])
	if err != nil {
		t.Fatalf("decode unknown apps item: %v", err)
	}
	got, err := view.Item(0)
	if err != nil {
		t.Fatalf("read unknown apps item: %v", err)
	}
	if got.Status != PidLookupUnknown || got.Pid != 4321 ||
		len(got.Comm.Bytes()) != 0 || len(got.CgroupPath.Bytes()) != 0 || len(got.CgroupName.Bytes()) != 0 {
		t.Fatalf("decoded unknown apps item = %+v", got)
	}

	bad := append([]byte(nil), resp[:total]...)
	bad[itemStart+AppsLookupItemHdr+1] = 'x'
	if _, err := DecodeAppsLookupResponse(bad); err != ErrMissingNul {
		t.Fatalf("decode missing unknown path NUL error = %v, want ErrMissingNul", err)
	}
}

func TestCgroupsLookupUnknownItemCanonicalLayout(t *testing.T) {
	path := []byte("/sys/fs/cgroup/bench/cg-001")
	var resp [128]byte
	builder := NewCgroupsLookupBuilder(resp[:], 1, 9)
	if err := builder.Add(CgroupLookupUnknownRetryLater, 0, path, nil, nil); err != nil {
		t.Fatalf("add unknown cgroups item: %v", err)
	}
	total := builder.Finish()
	itemStart := cgroupsLookupItemStart(t, resp[:total])
	item := resp[itemStart:total]
	wantSize := cgroupsLookupUnknownFixedBytes + len(path) + 1

	if got := len(item); got != wantSize {
		t.Fatalf("unknown item size = %d, want %d", got, wantSize)
	}
	if got := ne.Uint32(resp[CgroupsLookupRespHdr+4 : CgroupsLookupRespHdr+8]); got != uint32(wantSize) {
		t.Fatalf("directory item length = %d, want %d", got, wantSize)
	}
	if got := ne.Uint32(item[8:12]); got != CgroupsLookupItemHdr {
		t.Fatalf("path offset = %d, want %d", got, CgroupsLookupItemHdr)
	}
	wantNameOff := CgroupsLookupItemHdr + len(path) + 1
	if got := ne.Uint32(item[16:20]); got != uint32(wantNameOff) {
		t.Fatalf("name offset = %d, want %d", got, wantNameOff)
	}
	if item[CgroupsLookupItemHdr+len(path)] != 0 || item[wantNameOff] != 0 {
		t.Fatalf("unknown item NUL bytes are not canonical")
	}

	view, err := DecodeCgroupsLookupResponse(resp[:total])
	if err != nil {
		t.Fatalf("decode unknown cgroups item: %v", err)
	}
	got, err := view.Item(0)
	if err != nil {
		t.Fatalf("read unknown cgroups item: %v", err)
	}
	if got.Status != CgroupLookupUnknownRetryLater || got.Path.String() != string(path) ||
		len(got.Name.Bytes()) != 0 || got.LabelCount != 0 {
		t.Fatalf("decoded unknown cgroups item = %+v", got)
	}

	bad := append([]byte(nil), resp[:total]...)
	bad[itemStart+wantNameOff] = 'x'
	if _, err := DecodeCgroupsLookupResponse(bad); err != ErrMissingNul {
		t.Fatalf("decode missing unknown name NUL error = %v, want ErrMissingNul", err)
	}
}

func TestLookupInternalGuardCoverage(t *testing.T) {
	if _, ok := checkedU32Int(-1); ok {
		t.Fatalf("checkedU32Int(-1) succeeded")
	}
	if _, ok := checkedU32Int(int(uint64(^uint32(0)) + 1)); ok {
		t.Fatalf("checkedU32Int(uint32 max + 1) succeeded")
	}
	if _, ok := checkedU16Int(-1); ok {
		t.Fatalf("checkedU16Int(-1) succeeded")
	}
	if _, ok := checkedU16Int(int(^uint16(0)) + 1); ok {
		t.Fatalf("checkedU16Int(uint16 max + 1) succeeded")
	}
	if _, ok := checkedInt(uint64(maxIntValue()) + 1); ok {
		t.Fatalf("checkedInt(max + 1) succeeded")
	}
	if _, ok := checkedAddInt(-1, 0); ok {
		t.Fatalf("checkedAddInt negative succeeded")
	}
	if _, ok := checkedAddInt(maxIntValue(), 1); ok {
		t.Fatalf("checkedAddInt overflow succeeded")
	}
	if _, ok := checkedMulInt(-1, 1); ok {
		t.Fatalf("checkedMulInt negative succeeded")
	}
	if _, ok := checkedMulInt(maxIntValue(), 2); ok {
		t.Fatalf("checkedMulInt overflow succeeded")
	}
	if _, ok := checkedAlign8(-1); ok {
		t.Fatalf("checkedAlign8 negative succeeded")
	}
	if _, ok := checkedAlign8(maxIntValue() - 6); ok {
		t.Fatalf("checkedAlign8 overflow succeeded")
	}
	if _, err := lookupPayloadSlice([]byte{0}, -1, 0, 0); err != ErrOutOfBounds {
		t.Fatalf("lookupPayloadSlice negative error = %v, want ErrOutOfBounds", err)
	}
	if _, err := lookupPayloadSlice([]byte{0}, 0, 0, 2); err != ErrOutOfBounds {
		t.Fatalf("lookupPayloadSlice too long error = %v, want ErrOutOfBounds", err)
	}
	if err := validateLookupDir(make([]byte, 8), maxIntValue(), 1, 0, 0, -1); err != ErrBadItemCount {
		t.Fatalf("validateLookupDir bad item count error = %v, want ErrBadItemCount", err)
	}
	if err := validateLookupDir(make([]byte, 0), 0, 1, 0, 0, -1); err != ErrTruncated {
		t.Fatalf("validateLookupDir truncated error = %v, want ErrTruncated", err)
	}
	var dir [16]byte
	ne.PutUint32(dir[0:4], 8)
	ne.PutUint32(dir[4:8], 8)
	ne.PutUint32(dir[8:12], 0)
	ne.PutUint32(dir[12:16], 8)
	if err := validateLookupDir(dir[:], 0, 2, 32, 1, -1); err != ErrBadLayout {
		t.Fatalf("validateLookupDir overlap error = %v, want ErrBadLayout", err)
	}

	item := []byte{'a', 0, 'b', 0}
	if _, _, err := lookupString(item, 1, 0, 1); err != ErrOutOfBounds {
		t.Fatalf("lookupString below header error = %v, want ErrOutOfBounds", err)
	}
	if _, _, err := lookupString(item, 0, 3, 2); err != ErrOutOfBounds {
		t.Fatalf("lookupString oob error = %v, want ErrOutOfBounds", err)
	}
	if _, _, err := lookupString(item, 0, 0, 2); err != ErrMissingNul {
		t.Fatalf("lookupString missing nul error = %v, want ErrMissingNul", err)
	}
	if _, _, err := lookupString([]byte{'a', 0, 0}, 0, 0, 2); err != ErrBadLayout {
		t.Fatalf("lookupString interior nul error = %v, want ErrBadLayout", err)
	}

	if _, err := lookupLabelAt(make([]byte, LookupLabelEntrySize), 0, 1, -1, 0); err != ErrOutOfBounds {
		t.Fatalf("lookupLabelAt bad table offset error = %v, want ErrOutOfBounds", err)
	}

	labelItem := make([]byte, 52)
	ne.PutUint32(labelItem[32:36], 48)
	ne.PutUint32(labelItem[36:40], 1)
	ne.PutUint32(labelItem[40:44], 50)
	ne.PutUint32(labelItem[44:48], 1)
	labelItem[48] = 'k'
	labelItem[49] = 0
	labelItem[50] = 'v'
	labelItem[51] = 0
	if _, err := validateLabels(labelItem, 28, 0, 28); err != ErrBadLayout {
		t.Fatalf("validateLabels zero count wrong fixedEnd error = %v, want ErrBadLayout", err)
	}
	if _, err := validateLabels(labelItem, 28, 1, maxIntValue()-6); err != ErrOutOfBounds {
		t.Fatalf("validateLabels align overflow error = %v, want ErrOutOfBounds", err)
	}
	if _, err := validateLabels(labelItem, 28, 1, 60); err != ErrOutOfBounds {
		t.Fatalf("validateLabels table after end error = %v, want ErrOutOfBounds", err)
	}
	badPadding := append([]byte(nil), labelItem...)
	badPadding[28] = 1
	if _, err := validateLabels(badPadding, 28, 1, 28); err != ErrBadLayout {
		t.Fatalf("validateLabels bad padding error = %v, want ErrBadLayout", err)
	}
	if _, err := validateLabels(labelItem[:40], 28, 1, 28); err != ErrOutOfBounds {
		t.Fatalf("validateLabels table oob error = %v, want ErrOutOfBounds", err)
	}
	badKeyLen := append([]byte(nil), labelItem...)
	ne.PutUint32(badKeyLen[36:40], 0)
	if _, err := validateLabels(badKeyLen, 28, 1, 28); err != ErrBadLayout {
		t.Fatalf("validateLabels bad key length error = %v, want ErrBadLayout", err)
	}
	badKeyOff := append([]byte(nil), labelItem...)
	ne.PutUint32(badKeyOff[32:36], 49)
	if _, err := validateLabels(badKeyOff, 28, 1, 28); err != ErrBadLayout {
		t.Fatalf("validateLabels bad key offset error = %v, want ErrBadLayout", err)
	}
	badValueOff := append([]byte(nil), labelItem...)
	ne.PutUint32(badValueOff[40:44], 49)
	if _, err := validateLabels(badValueOff, 28, 1, 28); err != ErrBadLayout {
		t.Fatalf("validateLabels bad value offset error = %v, want ErrBadLayout", err)
	}
	badValueNul := append([]byte(nil), labelItem...)
	badValueNul[51] = '!'
	if _, err := validateLabels(badValueNul, 28, 1, 28); err != ErrMissingNul {
		t.Fatalf("validateLabels bad value nul error = %v, want ErrMissingNul", err)
	}
	extra := append([]byte(nil), labelItem...)
	extra = append(extra, 0)
	if _, err := validateLabels(extra, 28, 1, 28); err != ErrBadLayout {
		t.Fatalf("validateLabels extra byte error = %v, want ErrBadLayout", err)
	}

	var compact [80]byte
	ne.PutUint32(compact[16:20], 40)
	copy(compact[40:44], []byte{1, 2, 3, 4})
	if n := finishLookupResponse(compact[:], 16, 1, 44, 7); n != 28 {
		t.Fatalf("finishLookupResponse compact = %d, want 28", n)
	}
	var badFinish [80]byte
	ne.PutUint32(badFinish[16:20], 40)
	ne.PutUint32(badFinish[24:28], 32)
	if n := finishLookupResponse(badFinish[:], 16, 2, 48, 7); n != 0 {
		t.Fatalf("finishLookupResponse bad second offset = %d, want 0", n)
	}
	if n := finishLookupResponse(badFinish[:], maxIntValue(), 1, 0, 7); n != 0 {
		t.Fatalf("finishLookupResponse header overflow = %d, want 0", n)
	}
}

func TestLookupDecodeAndItemErrorCoverage(t *testing.T) {
	if _, err := DecodeCgroupsLookupResponse(make([]byte, CgroupsLookupRespHdr-1)); err != ErrTruncated {
		t.Fatalf("short cgroups response error = %v, want ErrTruncated", err)
	}
	cgResp := make([]byte, CgroupsLookupRespHdr+LookupDirEntrySize)
	ne.PutUint16(cgResp[0:2], 1)
	ne.PutUint32(cgResp[4:8], 1)
	ne.PutUint32(cgResp[CgroupsLookupRespHdr+4:CgroupsLookupRespHdr+8], CgroupsLookupItemHdr)
	if _, err := DecodeCgroupsLookupResponse(cgResp[:CgroupsLookupRespHdr]); err != ErrTruncated {
		t.Fatalf("cgroups response truncated dir error = %v, want ErrTruncated", err)
	}
	if _, err := DecodeCgroupsLookupResponse(cgResp); err != ErrOutOfBounds {
		t.Fatalf("cgroups response missing item error = %v, want ErrOutOfBounds", err)
	}
	cgShortItem := append([]byte(nil), cgResp...)
	ne.PutUint32(cgShortItem[CgroupsLookupRespHdr+4:CgroupsLookupRespHdr+8], CgroupsLookupItemHdr-1)
	cgShortItem = append(cgShortItem, make([]byte, CgroupsLookupItemHdr-1)...)
	if _, err := DecodeCgroupsLookupResponse(cgShortItem); err != ErrBadLayout {
		t.Fatalf("cgroups response short item error = %v, want ErrBadLayout", err)
	}
	if _, err := (&CgroupsLookupResponseView{ItemCount: 1, payload: cgResp}).Item(0); err != ErrOutOfBounds {
		t.Fatalf("manual cgroups item error = %v, want ErrOutOfBounds", err)
	}

	cgItem := validCgroupsLookupItemBytes(t)
	for _, tc := range []struct {
		name string
		edit func([]byte)
		want error
	}{
		{
			name: "short",
			edit: func(b []byte) {
				b = b[:CgroupsLookupItemHdr-1]
				copy(cgItem, b)
			},
			want: ErrTruncated,
		},
		{
			name: "bad reserved",
			edit: func(b []byte) {
				ne.PutUint16(b[6:8], 1)
			},
			want: ErrBadLayout,
		},
		{
			name: "path below header",
			edit: func(b []byte) {
				ne.PutUint32(b[8:12], 0)
			},
			want: ErrOutOfBounds,
		},
		{
			name: "name below header",
			edit: func(b []byte) {
				ne.PutUint32(b[16:20], 0)
			},
			want: ErrOutOfBounds,
		},
		{
			name: "label table missing",
			edit: func(b []byte) {
				ne.PutUint16(b[24:26], 2)
			},
			want: ErrOutOfBounds,
		},
	} {
		item := validCgroupsLookupItemBytes(t)
		if tc.name == "short" {
			if _, err := decodeCgroupsLookupItem(item[:CgroupsLookupItemHdr-1]); err != tc.want {
				t.Fatalf("decode cgroups item %s error = %v, want %v", tc.name, err, tc.want)
			}
			continue
		}
		tc.edit(item)
		if _, err := decodeCgroupsLookupItem(item); err != tc.want {
			t.Fatalf("decode cgroups item %s error = %v, want %v", tc.name, err, tc.want)
		}
	}

	if _, err := DecodeAppsLookupResponse(make([]byte, AppsLookupRespHdr-1)); err != ErrTruncated {
		t.Fatalf("short apps response error = %v, want ErrTruncated", err)
	}
	appsResp := make([]byte, AppsLookupRespHdr+LookupDirEntrySize)
	ne.PutUint16(appsResp[0:2], 1)
	ne.PutUint32(appsResp[4:8], 1)
	ne.PutUint32(appsResp[AppsLookupRespHdr+4:AppsLookupRespHdr+8], AppsLookupItemHdr)
	if _, err := DecodeAppsLookupResponse(appsResp[:AppsLookupRespHdr]); err != ErrTruncated {
		t.Fatalf("apps response truncated dir error = %v, want ErrTruncated", err)
	}
	if _, err := DecodeAppsLookupResponse(appsResp); err != ErrOutOfBounds {
		t.Fatalf("apps response missing item error = %v, want ErrOutOfBounds", err)
	}
	appsShortItem := append([]byte(nil), appsResp...)
	ne.PutUint32(appsShortItem[AppsLookupRespHdr+4:AppsLookupRespHdr+8], AppsLookupItemHdr-1)
	appsShortItem = append(appsShortItem, make([]byte, AppsLookupItemHdr-1)...)
	if _, err := DecodeAppsLookupResponse(appsShortItem); err != ErrBadLayout {
		t.Fatalf("apps response short item error = %v, want ErrBadLayout", err)
	}
	if _, err := (&AppsLookupResponseView{ItemCount: 1, payload: appsResp}).Item(0); err != ErrOutOfBounds {
		t.Fatalf("manual apps item error = %v, want ErrOutOfBounds", err)
	}

	for _, tc := range []struct {
		name string
		edit func([]byte)
		want error
	}{
		{
			name: "short",
			edit: nil,
			want: ErrTruncated,
		},
		{
			name: "bad reserved",
			edit: func(b []byte) {
				ne.PutUint32(b[20:24], 1)
			},
			want: ErrBadLayout,
		},
		{
			name: "comm below header",
			edit: func(b []byte) {
				ne.PutUint32(b[32:36], 0)
			},
			want: ErrOutOfBounds,
		},
		{
			name: "path below header",
			edit: func(b []byte) {
				ne.PutUint32(b[40:44], 0)
			},
			want: ErrOutOfBounds,
		},
		{
			name: "name below header",
			edit: func(b []byte) {
				ne.PutUint32(b[48:52], 0)
			},
			want: ErrOutOfBounds,
		},
		{
			name: "retry with orchestrator",
			edit: func(b []byte) {
				ne.PutUint16(b[6:8], AppsCgroupUnknownRetryLater)
			},
			want: ErrBadLayout,
		},
		{
			name: "permanent missing path",
			edit: func(b []byte) {
				ne.PutUint16(b[4:6], 0)
				ne.PutUint16(b[6:8], AppsCgroupUnknownPermanent)
				ne.PutUint32(b[44:48], 0)
				ne.PutUint32(b[52:56], 0)
				ne.PutUint16(b[56:58], 0)
			},
			want: ErrBadLayout,
		},
		{
			name: "label table missing",
			edit: func(b []byte) {
				ne.PutUint16(b[56:58], 2)
			},
			want: ErrOutOfBounds,
		},
	} {
		item := validAppsLookupItemBytes(t)
		if tc.name == "short" {
			if _, err := decodeAppsLookupItem(item[:AppsLookupItemHdr-1]); err != tc.want {
				t.Fatalf("decode apps item %s error = %v, want %v", tc.name, err, tc.want)
			}
			continue
		}
		tc.edit(item)
		if _, err := decodeAppsLookupItem(item); err != tc.want {
			t.Fatalf("decode apps item %s error = %v, want %v", tc.name, err, tc.want)
		}
	}
	_ = cgItem
}

func TestLookupResponseDecodeValidationCoverage(t *testing.T) {
	var resp [1024]byte

	cg := NewCgroupsLookupBuilder(resp[:], 1, 0)
	if err := cg.Add(CgroupLookupKnown, OrchestratorDocker, []byte("/x"), []byte("name"), nil); err != nil {
		t.Fatalf("add cgroups response: %v", err)
	}
	cgTotal := cg.Finish()
	cgItem := cgroupsLookupItemStart(t, resp[:])

	for _, tc := range []struct {
		name string
		edit func([]byte)
		want error
	}{
		{
			name: "bad flags",
			edit: func(b []byte) {
				ne.PutUint16(b[2:4], 1)
			},
			want: ErrBadLayout,
		},
		{
			name: "bad item layout",
			edit: func(b []byte) {
				ne.PutUint16(b[cgItem:cgItem+2], 2)
			},
			want: ErrBadLayout,
		},
		{
			name: "empty path",
			edit: func(b []byte) {
				ne.PutUint32(b[cgItem+12:cgItem+16], 0)
			},
			want: ErrBadLayout,
		},
		{
			name: "unknown with metadata",
			edit: func(b []byte) {
				ne.PutUint16(b[cgItem+2:cgItem+4], CgroupLookupUnknownRetryLater)
			},
			want: ErrBadLayout,
		},
		{
			name: "overlap",
			edit: func(b []byte) {
				ne.PutUint32(b[cgItem+16:cgItem+20], uint32(CgroupsLookupItemHdr+1))
				ne.PutUint32(b[cgItem+20:cgItem+24], 1)
			},
			want: ErrBadLayout,
		},
	} {
		bad := append([]byte(nil), resp[:cgTotal]...)
		tc.edit(bad)
		if _, err := DecodeCgroupsLookupResponse(bad); err != tc.want {
			t.Fatalf("decode cgroups response %s error = %v, want %v", tc.name, err, tc.want)
		}
	}

	apps := NewAppsLookupBuilder(resp[:], 1, 0)
	if err := apps.Add(
		PidLookupKnown,
		AppsCgroupKnown,
		OrchestratorDocker,
		1234, 1, 1000, 42,
		[]byte("nginx"),
		[]byte("/docker/abc"),
		[]byte("container-a"),
		nil,
	); err != nil {
		t.Fatalf("add apps response: %v", err)
	}
	appsTotal := apps.Finish()
	appsItem := appsLookupItemStart(t, resp[:])

	for _, tc := range []struct {
		name string
		edit func([]byte)
		want error
	}{
		{
			name: "bad flags",
			edit: func(b []byte) {
				ne.PutUint16(b[2:4], 1)
			},
			want: ErrBadLayout,
		},
		{
			name: "bad item layout",
			edit: func(b []byte) {
				ne.PutUint16(b[appsItem:appsItem+2], 2)
			},
			want: ErrBadLayout,
		},
		{
			name: "bad status",
			edit: func(b []byte) {
				ne.PutUint16(b[appsItem+2:appsItem+4], 99)
			},
			want: ErrBadLayout,
		},
		{
			name: "bad cgroup status",
			edit: func(b []byte) {
				ne.PutUint16(b[appsItem+6:appsItem+8], 99)
			},
			want: ErrBadLayout,
		},
		{
			name: "comm too long",
			edit: func(b []byte) {
				ne.PutUint32(b[appsItem+36:appsItem+40], 16)
			},
			want: ErrBadLayout,
		},
		{
			name: "unknown with data",
			edit: func(b []byte) {
				ne.PutUint16(b[appsItem+2:appsItem+4], PidLookupUnknown)
			},
			want: ErrBadLayout,
		},
		{
			name: "known empty comm",
			edit: func(b []byte) {
				ne.PutUint32(b[appsItem+36:appsItem+40], 0)
			},
			want: ErrBadLayout,
		},
		{
			name: "known empty path",
			edit: func(b []byte) {
				ne.PutUint32(b[appsItem+44:appsItem+48], 0)
			},
			want: ErrBadLayout,
		},
		{
			name: "host root with metadata",
			edit: func(b []byte) {
				ne.PutUint16(b[appsItem+6:appsItem+8], AppsCgroupHostRoot)
			},
			want: ErrBadLayout,
		},
		{
			name: "overlap",
			edit: func(b []byte) {
				ne.PutUint32(b[appsItem+40:appsItem+44], uint32(AppsLookupItemHdr+1))
				ne.PutUint32(b[appsItem+44:appsItem+48], 4)
			},
			want: ErrBadLayout,
		},
	} {
		bad := append([]byte(nil), resp[:appsTotal]...)
		tc.edit(bad)
		if _, err := DecodeAppsLookupResponse(bad); err != tc.want {
			t.Fatalf("decode apps response %s error = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestLookupDispatchCoverage(t *testing.T) {
	var req [128]byte
	var resp [512]byte

	reqLen, err := EncodeCgroupsLookupRequest([][]byte{[]byte("/x"), []byte("/y")}, req[:])
	if err != nil {
		t.Fatalf("encode cgroups dispatch request: %v", err)
	}
	n, err := DispatchCgroupsLookup(req[:reqLen], resp[:], func(request *CgroupsLookupRequestView, builder *CgroupsLookupBuilder) bool {
		for i := uint32(0); i < request.ItemCount; i++ {
			path, err := request.Item(i)
			if err != nil {
				return false
			}
			if err := builder.Add(CgroupLookupKnown, OrchestratorDocker, path.Bytes(), []byte("name"), nil); err != nil {
				return false
			}
		}
		return true
	})
	if err != nil || n == 0 {
		t.Fatalf("dispatch cgroups success = n %d err %v", n, err)
	}
	if _, err := DecodeCgroupsLookupResponse(resp[:n]); err != nil {
		t.Fatalf("decode dispatched cgroups response: %v", err)
	}
	n, err = DispatchCgroupsLookup(req[:reqLen], resp[:], func(*CgroupsLookupRequestView, *CgroupsLookupBuilder) bool {
		return false
	})
	if err != ErrBadLayout || n != 0 {
		t.Fatalf("dispatch cgroups handler false = n %d err %v", n, err)
	}
	n, err = DispatchCgroupsLookup(req[:reqLen], resp[:], func(*CgroupsLookupRequestView, *CgroupsLookupBuilder) bool {
		return true
	})
	if err != ErrBadItemCount || n != 0 {
		t.Fatalf("dispatch cgroups bad count = n %d err %v", n, err)
	}
	n, err = DispatchCgroupsLookup(req[:reqLen], resp[:], func(_ *CgroupsLookupRequestView, builder *CgroupsLookupBuilder) bool {
		_ = builder.Add(99, 0, []byte("/x"), nil, nil)
		return false
	})
	if err != ErrBadLayout || n != 0 {
		t.Fatalf("dispatch cgroups builder error = n %d err %v", n, err)
	}
	n, err = DispatchCgroupsLookup(req[:reqLen], resp[:], func(_ *CgroupsLookupRequestView, builder *CgroupsLookupBuilder) bool {
		_ = builder.Add(99, 0, []byte("/x"), nil, nil)
		return true
	})
	if err != ErrBadLayout || n != 0 {
		t.Fatalf("dispatch cgroups post-handler builder error = n %d err %v", n, err)
	}
	n, err = DispatchCgroupsLookup(req[:reqLen], resp[:], func(request *CgroupsLookupRequestView, builder *CgroupsLookupBuilder) bool {
		builder.itemCount = request.ItemCount
		builder.dataOffset = 0
		ne.PutUint32(builder.buf[CgroupsLookupRespHdr:CgroupsLookupRespHdr+4], 8)
		return true
	})
	if err != ErrOverflow || n != 0 {
		t.Fatalf("dispatch cgroups finish error = n %d err %v", n, err)
	}
	if n, err := DispatchCgroupsLookup(req[:CgroupsLookupReqHdr-1], resp[:], nil); err != ErrTruncated || n != 0 {
		t.Fatalf("dispatch cgroups bad request = n %d err %v", n, err)
	}

	reqLen, err = EncodeAppsLookupRequest([]uint32{1, 2}, req[:])
	if err != nil {
		t.Fatalf("encode apps dispatch request: %v", err)
	}
	n, err = DispatchAppsLookup(req[:reqLen], resp[:], func(request *AppsLookupRequestView, builder *AppsLookupBuilder) bool {
		for i := uint32(0); i < request.ItemCount; i++ {
			pid, err := request.Item(i)
			if err != nil {
				return false
			}
			if err := builder.Add(PidLookupKnown, AppsCgroupHostRoot, 0, pid, 0, 0, 0, []byte("proc"), nil, nil, nil); err != nil {
				return false
			}
		}
		return true
	})
	if err != nil || n == 0 {
		t.Fatalf("dispatch apps success = n %d err %v", n, err)
	}
	if _, err := DecodeAppsLookupResponse(resp[:n]); err != nil {
		t.Fatalf("decode dispatched apps response: %v", err)
	}
	n, err = DispatchAppsLookup(req[:reqLen], resp[:], func(*AppsLookupRequestView, *AppsLookupBuilder) bool {
		return false
	})
	if err != ErrBadLayout || n != 0 {
		t.Fatalf("dispatch apps handler false = n %d err %v", n, err)
	}
	n, err = DispatchAppsLookup(req[:reqLen], resp[:], func(*AppsLookupRequestView, *AppsLookupBuilder) bool {
		return true
	})
	if err != ErrBadItemCount || n != 0 {
		t.Fatalf("dispatch apps bad count = n %d err %v", n, err)
	}
	n, err = DispatchAppsLookup(req[:reqLen], resp[:], func(_ *AppsLookupRequestView, builder *AppsLookupBuilder) bool {
		_ = builder.Add(99, 0, 0, 1, 0, 0, 0, []byte("x"), nil, nil, nil)
		return false
	})
	if err != ErrBadLayout || n != 0 {
		t.Fatalf("dispatch apps builder error = n %d err %v", n, err)
	}
	n, err = DispatchAppsLookup(req[:reqLen], resp[:], func(_ *AppsLookupRequestView, builder *AppsLookupBuilder) bool {
		_ = builder.Add(99, 0, 0, 1, 0, 0, 0, []byte("x"), nil, nil, nil)
		return true
	})
	if err != ErrBadLayout || n != 0 {
		t.Fatalf("dispatch apps post-handler builder error = n %d err %v", n, err)
	}
	n, err = DispatchAppsLookup(req[:reqLen], resp[:], func(request *AppsLookupRequestView, builder *AppsLookupBuilder) bool {
		builder.itemCount = request.ItemCount
		builder.dataOffset = 0
		ne.PutUint32(builder.buf[AppsLookupRespHdr:AppsLookupRespHdr+4], 8)
		return true
	})
	if err != ErrOverflow || n != 0 {
		t.Fatalf("dispatch apps finish error = n %d err %v", n, err)
	}
	if n, err := DispatchAppsLookup(req[:AppsLookupReqHdr-1], resp[:], nil); err != ErrTruncated || n != 0 {
		t.Fatalf("dispatch apps bad request = n %d err %v", n, err)
	}
}
