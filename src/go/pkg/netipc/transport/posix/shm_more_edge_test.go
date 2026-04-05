//go:build linux

package posix

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func defaultShmLayout() (uint32, uint32, uint32, uint32) {
	reqCap := shmAlign64(128)
	respCap := shmAlign64(128)
	reqOff := shmAlign64(uint32(shmHeaderLen))
	respOff := shmAlign64(reqOff + reqCap)
	return reqOff, reqCap, respOff, respCap
}

func fillShmHeader(data []byte, ownerPID int32, ownerGen uint32, reqOff, reqCap, respOff, respCap uint32) {
	if len(data) < int(shmHeaderLen) {
		return
	}
	binary.NativeEndian.PutUint32(data[shmHeaderMagicOff:shmHeaderMagicOff+4], shmRegionMagic)
	binary.NativeEndian.PutUint16(data[shmHeaderVersionOff:shmHeaderVersionOff+2], shmRegionVersion)
	binary.NativeEndian.PutUint16(data[shmHeaderHeaderLenOff:shmHeaderHeaderLenOff+2], uint16(shmHeaderLen))
	binary.NativeEndian.PutUint32(data[shmHeaderOwnerPidOff:shmHeaderOwnerPidOff+4], uint32(ownerPID))
	binary.NativeEndian.PutUint32(data[shmHeaderOwnerGenOff:shmHeaderOwnerGenOff+4], ownerGen)
	binary.NativeEndian.PutUint32(data[shmHeaderReqOffOff:shmHeaderReqOffOff+4], reqOff)
	binary.NativeEndian.PutUint32(data[shmHeaderReqCapOff:shmHeaderReqCapOff+4], reqCap)
	binary.NativeEndian.PutUint32(data[shmHeaderRespOffOff:shmHeaderRespOffOff+4], respOff)
	binary.NativeEndian.PutUint32(data[shmHeaderRespCapOff:shmHeaderRespCapOff+4], respCap)
}

func writeRawShmRegionFile(t *testing.T, runDir, service string, sessionID uint64, size int, fill func([]byte)) string {
	t.Helper()
	if err := os.MkdirAll(runDir, 0700); err != nil {
		t.Fatalf("mkdir %s: %v", runDir, err)
	}

	path, err := buildShmPath(runDir, service, sessionID)
	if err != nil {
		t.Fatalf("build path: %v", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	if err := f.Truncate(int64(size)); err != nil {
		t.Fatalf("truncate %s: %v", path, err)
	}
	if size == 0 {
		return path
	}

	data := make([]byte, size)
	if fill != nil {
		fill(data)
	}
	if _, err := f.WriteAt(data, 0); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestShmOwnerAliveFalsePaths(t *testing.T) {
	if (&ShmContext{data: make([]byte, 8)}).OwnerAlive() {
		t.Fatal("short header should not be treated as alive")
	}

	runDir := t.TempDir()
	svc := "go_shm_owner_false"
	ctx, err := ShmServerCreate(runDir, svc, 1, 1024, 1024)
	if err != nil {
		t.Fatalf("server create: %v", err)
	}
	defer ctx.ShmDestroy()

	binary.NativeEndian.PutUint32(ctx.data[shmHeaderOwnerPidOff:shmHeaderOwnerPidOff+4], 0)
	if ctx.OwnerAlive() {
		t.Fatal("owner pid 0 should not be treated as alive")
	}

	binary.NativeEndian.PutUint32(ctx.data[shmHeaderOwnerPidOff:shmHeaderOwnerPidOff+4], uint32(int32(os.Getpid())))
	binary.NativeEndian.PutUint32(ctx.data[shmHeaderOwnerGenOff:shmHeaderOwnerGenOff+4], ctx.ownerGeneration+1)
	if ctx.OwnerAlive() {
		t.Fatal("generation mismatch should not be treated as alive")
	}

	ctx.ownerGeneration = 0
	if !ctx.OwnerAlive() {
		t.Fatal("legacy zero cached generation should skip generation check")
	}
}

func TestShmServerCreateRejectsLiveRegion(t *testing.T) {
	runDir := t.TempDir()
	svc := "go_shm_live_region"

	first, err := ShmServerCreate(runDir, svc, 7, 1024, 1024)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	defer first.ShmDestroy()

	_, err = ShmServerCreate(runDir, svc, 7, 1024, 1024)
	if !errors.Is(err, ErrShmOpen) {
		t.Fatalf("second create error = %v, want %v", err, ErrShmOpen)
	}
}

func TestShmServerCreateRecoversInvalidAndLegacyFiles(t *testing.T) {
	runDir := t.TempDir()

	t.Run("tiny-invalid-file", func(t *testing.T) {
		svc := "go_shm_recover_tiny"
		writeRawShmRegionFile(t, runDir, svc, 1, 8, nil)

		ctx, err := ShmServerCreate(runDir, svc, 1, 1024, 1024)
		if err != nil {
			t.Fatalf("create after tiny stale file: %v", err)
		}
		defer ctx.ShmDestroy()
	})

	t.Run("zero-generation-legacy-file", func(t *testing.T) {
		svc := "go_shm_recover_legacy"
		reqOff, reqCap, respOff, respCap := defaultShmLayout()
		writeRawShmRegionFile(t, runDir, svc, 2, int(respOff+respCap), func(data []byte) {
			fillShmHeader(data, int32(os.Getpid()), 0, reqOff, reqCap, respOff, respCap)
		})

		ctx, err := ShmServerCreate(runDir, svc, 2, 1024, 1024)
		if err != nil {
			t.Fatalf("create after zero-generation stale file: %v", err)
		}
		defer ctx.ShmDestroy()
	})
}

func TestShmClientAttachRejectsMalformedHeaders(t *testing.T) {
	runDir := t.TempDir()
	reqOff, reqCap, respOff, respCap := defaultShmLayout()
	validSize := int(respOff + respCap)

	tests := []struct {
		name string
		size int
		fill func([]byte)
		want error
	}{
		{
			name: "file-too-small",
			size: 8,
			fill: nil,
			want: ErrShmNotReady,
		},
		{
			name: "bad-magic",
			size: validSize,
			fill: func(data []byte) {
				fillShmHeader(data, int32(os.Getpid()), 1, reqOff, reqCap, respOff, respCap)
				binary.NativeEndian.PutUint32(data[shmHeaderMagicOff:shmHeaderMagicOff+4], 0)
			},
			want: ErrShmBadMagic,
		},
		{
			name: "bad-version",
			size: validSize,
			fill: func(data []byte) {
				fillShmHeader(data, int32(os.Getpid()), 1, reqOff, reqCap, respOff, respCap)
				binary.NativeEndian.PutUint16(data[shmHeaderVersionOff:shmHeaderVersionOff+2], shmRegionVersion+1)
			},
			want: ErrShmBadVersion,
		},
		{
			name: "bad-header-length",
			size: validSize,
			fill: func(data []byte) {
				fillShmHeader(data, int32(os.Getpid()), 1, reqOff, reqCap, respOff, respCap)
				binary.NativeEndian.PutUint16(data[shmHeaderHeaderLenOff:shmHeaderHeaderLenOff+2], 32)
			},
			want: ErrShmBadHeader,
		},
		{
			name: "bad-layout-alignment",
			size: validSize,
			fill: func(data []byte) {
				fillShmHeader(data, int32(os.Getpid()), 1, reqOff+1, reqCap, respOff, respCap)
			},
			want: ErrShmBadSize,
		},
		{
			name: "declared-region-beyond-file-size",
			size: int(shmHeaderLen) + 64,
			fill: func(data []byte) {
				fillShmHeader(data, int32(os.Getpid()), 1, reqOff, reqCap, respOff, respCap)
			},
			want: ErrShmBadSize,
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := "go_shm_attach_bad"
			writeRawShmRegionFile(t, runDir, svc, uint64(i+1), tc.size, tc.fill)

			_, err := ShmClientAttach(runDir, svc, uint64(i+1))
			if !errors.Is(err, tc.want) {
				t.Fatalf("attach error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestShmCreateAndAttachRejectInvalidServiceName(t *testing.T) {
	runDir := t.TempDir()

	if _, err := ShmServerCreate(runDir, "bad/name", 1, 1024, 1024); !errors.Is(err, ErrShmBadParam) {
		t.Fatalf("ShmServerCreate invalid service name = %v, want %v", err, ErrShmBadParam)
	}
	if _, err := ShmClientAttach(runDir, "bad/name", 1); !errors.Is(err, ErrShmBadParam) {
		t.Fatalf("ShmClientAttach invalid service name = %v, want %v", err, ErrShmBadParam)
	}
}

func TestShmSendBadParamGuards(t *testing.T) {
	if err := (&ShmContext{}).ShmSend([]byte{1}); !errors.Is(err, ErrShmBadParam) {
		t.Fatalf("ShmSend nil context = %v, want %v", err, ErrShmBadParam)
	}

	storeLenCtx := &ShmContext{
		role:            ShmRoleClient,
		data:            make([]byte, 39),
		requestCapacity: 64,
	}
	if err := storeLenCtx.ShmSend([]byte{1}); !errors.Is(err, ErrShmBadParam) {
		t.Fatalf("ShmSend short backing slice for len store = %v, want %v", err, ErrShmBadParam)
	}

	addSeqCtx := &ShmContext{
		role:            ShmRoleClient,
		data:            make([]byte, 48),
		requestCapacity: 64,
	}
	if err := addSeqCtx.ShmSend([]byte{1}); !errors.Is(err, ErrShmBadParam) {
		t.Fatalf("ShmSend short backing slice for seq add = %v, want %v", err, ErrShmBadParam)
	}

	addSignalCtx := &ShmContext{
		role:            ShmRoleClient,
		data:            make([]byte, 56),
		requestCapacity: 64,
	}
	if err := addSignalCtx.ShmSend([]byte{1}); !errors.Is(err, ErrShmBadParam) {
		t.Fatalf("ShmSend short backing slice for signal add = %v, want %v", err, ErrShmBadParam)
	}
}

func TestShmReceiveBadParamAndTimeoutPaths(t *testing.T) {
	if _, err := (&ShmContext{}).ShmReceive(make([]byte, 8), 1); !errors.Is(err, ErrShmBadParam) {
		t.Fatalf("ShmReceive nil context = %v, want %v", err, ErrShmBadParam)
	}

	if _, err := (&ShmContext{data: make([]byte, 8)}).ShmReceive(nil, 1); !errors.Is(err, ErrShmBadParam) {
		t.Fatalf("ShmReceive empty buffer = %v, want %v", err, ErrShmBadParam)
	}

	spinLoadSeqCtx := &ShmContext{
		role:             ShmRoleClient,
		data:             make([]byte, 8),
		responseCapacity: 64,
		SpinTries:        1,
	}
	if _, err := spinLoadSeqCtx.ShmReceive(make([]byte, 8), 1); !errors.Is(err, ErrShmBadParam) {
		t.Fatalf("ShmReceive spin-phase seq load = %v, want %v", err, ErrShmBadParam)
	}

	spinLoadLenCtx := &ShmContext{
		role:             ShmRoleClient,
		data:             make([]byte, 48),
		responseCapacity: 64,
		SpinTries:        1,
	}
	binary.NativeEndian.PutUint64(
		spinLoadLenCtx.data[shmHeaderRespSeqOff:shmHeaderRespSeqOff+8],
		1,
	)
	if _, err := spinLoadLenCtx.ShmReceive(make([]byte, 8), 1); !errors.Is(err, ErrShmBadParam) {
		t.Fatalf("ShmReceive spin-phase msg_len load = %v, want %v", err, ErrShmBadParam)
	}

	futexLoadSignalCtx := &ShmContext{
		role:             ShmRoleClient,
		data:             make([]byte, 8),
		responseCapacity: 64,
		SpinTries:        0,
	}
	if _, err := futexLoadSignalCtx.ShmReceive(make([]byte, 8), 1); !errors.Is(err, ErrShmBadParam) {
		t.Fatalf("ShmReceive futex-phase signal load = %v, want %v", err, ErrShmBadParam)
	}

	futexLoadSeqCtx := &ShmContext{
		role:             ShmRoleClient,
		data:             make([]byte, 48),
		responseCapacity: 64,
		SpinTries:        0,
	}
	if _, err := futexLoadSeqCtx.ShmReceive(make([]byte, 8), 1); !errors.Is(err, ErrShmBadParam) {
		t.Fatalf("ShmReceive futex-phase seq load = %v, want %v", err, ErrShmBadParam)
	}

	runDir := t.TempDir()
	svc := "go_shm_timeout"
	srv, err := ShmServerCreate(runDir, svc, 11, 1024, 1024)
	if err != nil {
		t.Fatalf("ShmServerCreate timeout fixture failed: %v", err)
	}
	defer srv.ShmDestroy()

	client, err := ShmClientAttach(runDir, svc, 11)
	if err != nil {
		t.Fatalf("ShmClientAttach timeout fixture failed: %v", err)
	}
	defer client.ShmClose()

	buf := make([]byte, 64)
	if _, err := srv.ShmReceive(buf, 1); !errors.Is(err, ErrShmTimeout) {
		t.Fatalf("server ShmReceive timeout = %v, want %v", err, ErrShmTimeout)
	}
	if _, err := client.ShmReceive(buf, 1); !errors.Is(err, ErrShmTimeout) {
		t.Fatalf("client ShmReceive timeout = %v, want %v", err, ErrShmTimeout)
	}
}

func TestCheckShmStaleVariants(t *testing.T) {
	runDir := t.TempDir()
	reqOff, reqCap, respOff, respCap := defaultShmLayout()
	validSize := int(respOff + respCap)

	missingPath := filepath.Join(runDir, "missing.ipcshm")
	if got := checkShmStale(missingPath); got != shmStaleNotExist {
		t.Fatalf("missing path result = %v, want %v", got, shmStaleNotExist)
	}

	tinyPath := writeRawShmRegionFile(t, runDir, "go_shm_check_tiny", 1, 8, nil)
	if got := checkShmStale(tinyPath); got != shmStaleInvalid {
		t.Fatalf("tiny file result = %v, want %v", got, shmStaleInvalid)
	}
	if _, err := os.Stat(tinyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("tiny file should be removed, stat err = %v", err)
	}

	badMagicPath := writeRawShmRegionFile(t, runDir, "go_shm_check_magic", 2, validSize, func(data []byte) {
		fillShmHeader(data, int32(os.Getpid()), 1, reqOff, reqCap, respOff, respCap)
		binary.NativeEndian.PutUint32(data[shmHeaderMagicOff:shmHeaderMagicOff+4], 0)
	})
	if got := checkShmStale(badMagicPath); got != shmStaleInvalid {
		t.Fatalf("bad magic result = %v, want %v", got, shmStaleInvalid)
	}

	livePath := writeRawShmRegionFile(t, runDir, "go_shm_check_live", 3, validSize, func(data []byte) {
		fillShmHeader(data, int32(os.Getpid()), 7, reqOff, reqCap, respOff, respCap)
	})
	if got := checkShmStale(livePath); got != shmStaleLive {
		t.Fatalf("live path result = %v, want %v", got, shmStaleLive)
	}
	if _, err := os.Stat(livePath); err != nil {
		t.Fatalf("live file should remain, stat err = %v", err)
	}

	legacyPath := writeRawShmRegionFile(t, runDir, "go_shm_check_legacy", 4, validSize, func(data []byte) {
		fillShmHeader(data, int32(os.Getpid()), 0, reqOff, reqCap, respOff, respCap)
	})
	if got := checkShmStale(legacyPath); got != shmStaleRecovered {
		t.Fatalf("legacy path result = %v, want %v", got, shmStaleRecovered)
	}
	if _, err := os.Stat(legacyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy stale file should be removed, stat err = %v", err)
	}

	unreadablePath := writeRawShmRegionFile(t, runDir, "go_shm_check_unreadable", 5, validSize, func(data []byte) {
		fillShmHeader(data, int32(os.Getpid()), 1, reqOff, reqCap, respOff, respCap)
	})
	if err := os.Chmod(unreadablePath, 0); err != nil {
		t.Fatalf("chmod unreadable stale file: %v", err)
	}
	if got := checkShmStale(unreadablePath); got != shmStaleInvalid {
		t.Fatalf("unreadable path result = %v, want %v", got, shmStaleInvalid)
	}
	if _, err := os.Stat(unreadablePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unreadable stale file should be removed, stat err = %v", err)
	}

	dirPath, err := buildShmPath(runDir, "go_shm_check_dir", 6)
	if err != nil {
		t.Fatalf("build dir path: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dirPath, "keep"), 0700); err != nil {
		t.Fatalf("mkdir stale dir: %v", err)
	}
	if got := checkShmStale(dirPath); got != shmStaleInvalid {
		t.Fatalf("directory path result = %v, want %v", got, shmStaleInvalid)
	}
	if info, err := os.Stat(dirPath); err != nil || !info.IsDir() {
		t.Fatalf("non-empty stale directory should remain, stat err = %v", err)
	}
}

func TestShmServerCreateFailsWhenObstructionSurvivesRecovery(t *testing.T) {
	runDir := t.TempDir()
	svc := "go_shm_create_blocked"
	path, err := buildShmPath(runDir, svc, 7)
	if err != nil {
		t.Fatalf("build path: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(path, "keep"), 0700); err != nil {
		t.Fatalf("mkdir obstructing directory: %v", err)
	}

	_, err = ShmServerCreate(runDir, svc, 7, 1024, 1024)
	if !errors.Is(err, ErrShmOpen) {
		t.Fatalf("ShmServerCreate blocked path error = %v, want %v", err, ErrShmOpen)
	}
	if info, statErr := os.Stat(path); statErr != nil || !info.IsDir() {
		t.Fatalf("obstructing directory should remain after failed recovery, stat err = %v", statErr)
	}
}

func TestShmCleanupStaleMixedEntries(t *testing.T) {
	runDir := t.TempDir()
	reqOff, reqCap, respOff, respCap := defaultShmLayout()
	validSize := int(respOff + respCap)
	svc := "go_shm_cleanup"

	live, err := ShmServerCreate(runDir, svc, 1, 1024, 1024)
	if err != nil {
		t.Fatalf("live server create: %v", err)
	}
	defer live.ShmDestroy()

	tinyPath := writeRawShmRegionFile(t, runDir, svc, 2, 8, nil)
	legacyPath := writeRawShmRegionFile(t, runDir, svc, 3, validSize, func(data []byte) {
		fillShmHeader(data, int32(os.Getpid()), 0, reqOff, reqCap, respOff, respCap)
	})
	unrelatedPath := filepath.Join(runDir, "other-file.ipcshm")
	if err := os.WriteFile(unrelatedPath, []byte("keep"), 0600); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}
	matchingDir := filepath.Join(runDir, svc+"-dir.ipcshm")
	if err := os.Mkdir(matchingDir, 0700); err != nil {
		t.Fatalf("mkdir matching dir: %v", err)
	}

	ShmCleanupStale(runDir, svc)

	if _, err := os.Stat(live.path); err != nil {
		t.Fatalf("live region should remain, stat err = %v", err)
	}
	if _, err := os.Stat(tinyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("tiny stale file should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(legacyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy stale file should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(unrelatedPath); err != nil {
		t.Fatalf("unrelated file should remain, stat err = %v", err)
	}
	if info, err := os.Stat(matchingDir); err != nil || !info.IsDir() {
		t.Fatalf("matching directory should remain, stat err = %v", err)
	}
}

func TestShmCleanupStaleMissingDirAndUnrelatedFile(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "missing")
	ShmCleanupStale(missingDir, "go_shm_missing")

	runDir := t.TempDir()
	unrelatedPath := filepath.Join(runDir, "go_shm_other-note.txt")
	if err := os.WriteFile(unrelatedPath, []byte("x"), 0600); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}

	ShmCleanupStale(runDir, "go_shm_other")

	if _, err := os.Stat(unrelatedPath); err != nil {
		t.Fatalf("unrelated file should remain after cleanup, stat err = %v", err)
	}
}
