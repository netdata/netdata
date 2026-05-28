//go:build windows

package windows

import (
	"encoding/binary"
	"errors"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

type shmReceiveResult struct {
	payload []byte
	err     error
}

func installWinShmOneShotFault(t *testing.T, target *winShmProcCall, failOnCall int, errno syscall.Errno) {
	t.Helper()

	orig := *target
	callCount := 0
	*target = func(a ...uintptr) (uintptr, uintptr, error) {
		callCount++
		if callCount == failOnCall {
			return 0, 0, errno
		}
		return orig(a...)
	}
	t.Cleanup(func() {
		*target = orig
	})
}

func TestWinShmHelpers(t *testing.T) {
	if got := winShmAlignCacheline(1); got != 64 {
		t.Fatalf("winShmAlignCacheline(1) = %d, want 64", got)
	}
	if got := winShmAlignCacheline(64); got != 64 {
		t.Fatalf("winShmAlignCacheline(64) = %d, want 64", got)
	}
	if got := winShmAlignCacheline(65); got != 128 {
		t.Fatalf("winShmAlignCacheline(65) = %d, want 128", got)
	}

	if err := validateWinShmProfile(0); !errors.Is(err, ErrWinShmBadParam) {
		t.Fatalf("validateWinShmProfile(0) = %v, want ErrWinShmBadParam", err)
	}
	if err := validateWinShmProfile(WinShmProfileHybrid); err != nil {
		t.Fatalf("validateWinShmProfile(hybrid) failed: %v", err)
	}
	if err := validateWinShmProfile(WinShmProfileBusywait); err != nil {
		t.Fatalf("validateWinShmProfile(busywait) failed: %v", err)
	}

	h1 := computeShmHash("run", "svc", 123)
	h2 := computeShmHash("run", "svc", 123)
	h3 := computeShmHash("run", "svc", 124)
	if h1 != h2 {
		t.Fatalf("computeShmHash not deterministic: %d != %d", h1, h2)
	}
	if h1 == h3 {
		t.Fatal("computeShmHash should change when auth token changes")
	}

	name, err := buildWinShmObjectName(h1, "svc", WinShmProfileHybrid, 7, "mapping")
	if err != nil {
		t.Fatalf("buildWinShmObjectName failed: %v", err)
	}
	if got := syscall.UTF16ToString(name); !strings.Contains(got, "svc") {
		t.Fatalf("object name %q does not contain service name", got)
	}

	tooLong := strings.Repeat("a", 260)
	if _, err := buildWinShmObjectName(h1, tooLong, WinShmProfileHybrid, 7, "mapping"); !errors.Is(err, ErrWinShmBadParam) {
		t.Fatalf("buildWinShmObjectName long name = %v, want ErrWinShmBadParam", err)
	}
}

func TestWinShmCreateAttachAndCloseValidation(t *testing.T) {
	runDir := t.TempDir()
	service := "validation"
	const authToken uint64 = 0x5678
	const sessionID uint64 = 9

	if _, err := WinShmServerCreate(runDir, "bad/name", authToken, sessionID, WinShmProfileHybrid, 4096, 4096); err == nil {
		t.Fatal("WinShmServerCreate with invalid service name should fail")
	}
	if _, err := WinShmServerCreate(runDir, service, authToken, sessionID, 0, 4096, 4096); !errors.Is(err, ErrWinShmBadParam) {
		t.Fatalf("WinShmServerCreate invalid profile = %v, want ErrWinShmBadParam", err)
	}
	if _, err := WinShmClientAttach(runDir, service, authToken, sessionID, 0); !errors.Is(err, ErrWinShmBadParam) {
		t.Fatalf("WinShmClientAttach invalid profile = %v, want ErrWinShmBadParam", err)
	}
	if _, err := WinShmClientAttach(runDir, service, authToken, sessionID, WinShmProfileHybrid); err == nil {
		t.Fatal("WinShmClientAttach without mapping should fail")
	}

	server, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096)
	if err != nil {
		t.Fatalf("WinShmServerCreate failed: %v", err)
	}
	defer server.WinShmDestroy()

	client, err := WinShmClientAttach(runDir, service, authToken, sessionID, WinShmProfileHybrid)
	if err != nil {
		t.Fatalf("WinShmClientAttach failed: %v", err)
	}
	defer client.WinShmClose()

	if server.GetRole() != WinShmRoleServer {
		t.Fatalf("server role = %d, want %d", server.GetRole(), WinShmRoleServer)
	}
	if client.GetRole() != WinShmRoleClient {
		t.Fatalf("client role = %d, want %d", client.GetRole(), WinShmRoleClient)
	}
}

func TestWinShmCreateAttachRejectsLongObjectName(t *testing.T) {
	runDir := t.TempDir()
	service := strings.Repeat("a", 220)
	const authToken uint64 = 0x9988
	const sessionID uint64 = 23

	if _, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096); !errors.Is(err, ErrWinShmBadParam) {
		t.Fatalf("WinShmServerCreate(long name) = %v, want ErrWinShmBadParam", err)
	}
	if _, err := WinShmClientAttach(runDir, service, authToken, sessionID, WinShmProfileHybrid); !errors.Is(err, ErrWinShmBadParam) {
		t.Fatalf("WinShmClientAttach(long name) = %v, want ErrWinShmBadParam", err)
	}
}

func TestWinShmServerCreateRejectsExistingObjects(t *testing.T) {
	runDir := t.TempDir()
	service := "addr-in-use"
	const authToken uint64 = 0x445566
	const sessionID uint64 = 25

	server, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096)
	if err != nil {
		t.Fatalf("WinShmServerCreate failed: %v", err)
	}
	defer server.WinShmDestroy()

	if _, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096); !errors.Is(err, ErrWinShmAddrInUse) {
		t.Fatalf("second WinShmServerCreate = %v, want ErrWinShmAddrInUse", err)
	}
}

func TestWinShmServerCreateRejectsLateEventNameOverflow(t *testing.T) {
	runDir := t.TempDir()
	const authToken uint64 = 0x9989
	const sessionID uint64 = 24

	t.Run("req_event name overflow", func(t *testing.T) {
		// 195 characters keeps the mapping name below the Win32 object-name
		// limit but pushes req_event over it.
		service := strings.Repeat("b", 195)
		if _, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096); !errors.Is(err, ErrWinShmBadParam) {
			t.Fatalf("WinShmServerCreate(req_event overflow) = %v, want ErrWinShmBadParam", err)
		}
	})

	t.Run("resp_event name overflow", func(t *testing.T) {
		// 194 characters still allows req_event creation, but resp_event
		// crosses the object-name limit and exercises the cleanup path.
		service := strings.Repeat("c", 194)
		if _, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096); !errors.Is(err, ErrWinShmBadParam) {
			t.Fatalf("WinShmServerCreate(resp_event overflow) = %v, want ErrWinShmBadParam", err)
		}
	})
}

func TestWinShmClientAttachRejectsCorruptHeader(t *testing.T) {
	cases := []struct {
		name   string
		want   error
		mutate func([]byte)
	}{
		{
			name: "bad magic",
			want: ErrWinShmBadMagic,
			mutate: func(data []byte) {
				binary.NativeEndian.PutUint32(data[wshOFFMagic:], 0)
			},
		},
		{
			name: "bad version",
			want: ErrWinShmBadVersion,
			mutate: func(data []byte) {
				binary.NativeEndian.PutUint32(data[wshOFFVersion:], winShmVersion+1)
			},
		},
		{
			name: "bad header len",
			want: ErrWinShmBadHeader,
			mutate: func(data []byte) {
				binary.NativeEndian.PutUint32(data[wshOFFHeaderLen:], winShmHeaderLen+64)
			},
		},
		{
			name: "bad profile",
			want: ErrWinShmBadProfile,
			mutate: func(data []byte) {
				binary.NativeEndian.PutUint32(data[wshOFFProfile:], WinShmProfileBusywait)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runDir := t.TempDir()
			service := "corrupt-header"
			const authToken uint64 = 0x123456
			const sessionID uint64 = 15

			server, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096)
			if err != nil {
				t.Fatalf("WinShmServerCreate failed: %v", err)
			}
			defer server.WinShmDestroy()

			data := unsafe.Slice((*byte)(unsafe.Pointer(server.base)), server.size)
			tc.mutate(data)

			_, err = WinShmClientAttach(runDir, service, authToken, sessionID, WinShmProfileHybrid)
			if !errors.Is(err, tc.want) {
				t.Fatalf("WinShmClientAttach error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestWinShmClientAttachFailsWhenEventsAreMissing(t *testing.T) {
	runDir := t.TempDir()
	service := "missing-events"
	const authToken uint64 = 0x556677

	t.Run("req_event missing", func(t *testing.T) {
		const sessionID uint64 = 31

		server, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096)
		if err != nil {
			t.Fatalf("WinShmServerCreate failed: %v", err)
		}
		defer server.WinShmDestroy()

		if err := syscall.CloseHandle(server.reqEvent); err != nil {
			t.Fatalf("CloseHandle(reqEvent) failed: %v", err)
		}
		server.reqEvent = syscall.InvalidHandle

		_, err = WinShmClientAttach(runDir, service, authToken, sessionID, WinShmProfileHybrid)
		if err == nil || !errors.Is(err, ErrWinShmOpenEvent) {
			t.Fatalf("WinShmClientAttach missing req_event = %v, want ErrWinShmOpenEvent", err)
		}
	})

	t.Run("resp_event missing", func(t *testing.T) {
		const sessionID uint64 = 33

		server, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096)
		if err != nil {
			t.Fatalf("WinShmServerCreate failed: %v", err)
		}
		defer server.WinShmDestroy()

		if err := syscall.CloseHandle(server.respEvent); err != nil {
			t.Fatalf("CloseHandle(respEvent) failed: %v", err)
		}
		server.respEvent = syscall.InvalidHandle

		_, err = WinShmClientAttach(runDir, service, authToken, sessionID, WinShmProfileHybrid)
		if err == nil || !errors.Is(err, ErrWinShmOpenEvent) {
			t.Fatalf("WinShmClientAttach missing resp_event = %v, want ErrWinShmOpenEvent", err)
		}
	})
}

func TestWinShmServerCreateWin32Failures(t *testing.T) {
	cases := []struct {
		name       string
		target     *winShmProcCall
		failOnCall int
		errno      syscall.Errno
		wantErr    error
		wantText   string
	}{
		{
			name:       "CreateFileMappingW",
			target:     &winShmCreateFileMappingW,
			failOnCall: 1,
			errno:      syscall.Errno(5),
			wantErr:    ErrWinShmCreateMapping,
		},
		{
			name:       "MapViewOfFile",
			target:     &winShmMapViewOfFile,
			failOnCall: 1,
			errno:      syscall.Errno(6),
			wantErr:    ErrWinShmMapView,
		},
		{
			name:       "CreateEventW req_event",
			target:     &winShmCreateEventW,
			failOnCall: 1,
			errno:      syscall.Errno(7),
			wantErr:    ErrWinShmCreateEvent,
			wantText:   "req_event",
		},
		{
			name:       "CreateEventW resp_event",
			target:     &winShmCreateEventW,
			failOnCall: 2,
			errno:      syscall.Errno(8),
			wantErr:    ErrWinShmCreateEvent,
			wantText:   "resp_event",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runDir := t.TempDir()
			service := "fault-create"
			const authToken uint64 = 0x7711
			const sessionID uint64 = 61

			installWinShmOneShotFault(t, tc.target, tc.failOnCall, tc.errno)

			_, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("WinShmServerCreate injected fault = %v, want %v", err, tc.wantErr)
			}
			if tc.wantText != "" && !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("WinShmServerCreate error %q does not mention %q", err, tc.wantText)
			}

			server, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096)
			if err != nil {
				t.Fatalf("WinShmServerCreate recovery failed: %v", err)
			}
			server.WinShmDestroy()
		})
	}
}

func TestWinShmClientAttachWin32Failures(t *testing.T) {
	cases := []struct {
		name       string
		target     *winShmProcCall
		failOnCall int
		errno      syscall.Errno
		wantErr    error
		wantText   string
	}{
		{
			name:       "OpenFileMappingW",
			target:     &winShmOpenFileMappingW,
			failOnCall: 1,
			errno:      syscall.Errno(9),
			wantErr:    ErrWinShmOpenMapping,
		},
		{
			name:       "MapViewOfFile",
			target:     &winShmMapViewOfFile,
			failOnCall: 1,
			errno:      syscall.Errno(10),
			wantErr:    ErrWinShmMapView,
		},
		{
			name:       "OpenEventW req_event",
			target:     &winShmOpenEventW,
			failOnCall: 1,
			errno:      syscall.Errno(11),
			wantErr:    ErrWinShmOpenEvent,
			wantText:   "req_event",
		},
		{
			name:       "OpenEventW resp_event",
			target:     &winShmOpenEventW,
			failOnCall: 2,
			errno:      syscall.Errno(12),
			wantErr:    ErrWinShmOpenEvent,
			wantText:   "resp_event",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runDir := t.TempDir()
			service := "fault-attach"
			const authToken uint64 = 0x7712
			const sessionID uint64 = 62

			server, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096)
			if err != nil {
				t.Fatalf("WinShmServerCreate failed: %v", err)
			}
			defer server.WinShmDestroy()

			installWinShmOneShotFault(t, tc.target, tc.failOnCall, tc.errno)

			_, err = WinShmClientAttach(runDir, service, authToken, sessionID, WinShmProfileHybrid)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("WinShmClientAttach injected fault = %v, want %v", err, tc.wantErr)
			}
			if tc.wantText != "" && !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("WinShmClientAttach error %q does not mention %q", err, tc.wantText)
			}

			client, err := WinShmClientAttach(runDir, service, authToken, sessionID, WinShmProfileHybrid)
			if err != nil {
				t.Fatalf("WinShmClientAttach recovery failed: %v", err)
			}
			client.WinShmClose()
		})
	}
}

func TestWinShmSendReceiveValidation(t *testing.T) {
	runDir := t.TempDir()
	service := "validation-io"
	const authToken uint64 = 0x6789
	const sessionID uint64 = 11

	server, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096)
	if err != nil {
		t.Fatalf("WinShmServerCreate failed: %v", err)
	}
	defer server.WinShmDestroy()

	client, err := WinShmClientAttach(runDir, service, authToken, sessionID, WinShmProfileHybrid)
	if err != nil {
		t.Fatalf("WinShmClientAttach failed: %v", err)
	}
	defer client.WinShmClose()

	if err := server.WinShmSend(nil); !errors.Is(err, ErrWinShmBadParam) {
		t.Fatalf("WinShmSend(nil) = %v, want ErrWinShmBadParam", err)
	}
	if _, err := client.WinShmReceive(nil, 10); !errors.Is(err, ErrWinShmBadParam) {
		t.Fatalf("WinShmReceive(nil) = %v, want ErrWinShmBadParam", err)
	}

	tooLarge := make([]byte, server.responseCapacity+1)
	if err := server.WinShmSend(tooLarge); !errors.Is(err, ErrWinShmMsgTooLarge) {
		t.Fatalf("WinShmSend(tooLarge) = %v, want ErrWinShmMsgTooLarge", err)
	}

	msg := []byte("0123456789")
	if err := server.WinShmSend(msg); err != nil {
		t.Fatalf("WinShmSend failed: %v", err)
	}

	smallBuf := make([]byte, 4)
	n, err := client.WinShmReceive(smallBuf, 1000)
	if !errors.Is(err, ErrWinShmMsgTooLarge) {
		t.Fatalf("WinShmReceive(smallBuf) = %v, want ErrWinShmMsgTooLarge", err)
	}
	if n != len(msg) {
		t.Fatalf("WinShmReceive reported len %d, want %d", n, len(msg))
	}
}

func TestWinShmReceiveDetectsPeerClosed(t *testing.T) {
	runDir := t.TempDir()
	service := "peer-closed"
	const authToken uint64 = 0x789a
	const sessionID uint64 = 13

	server, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096)
	if err != nil {
		t.Fatalf("WinShmServerCreate failed: %v", err)
	}
	defer server.WinShmDestroy()

	client, err := WinShmClientAttach(runDir, service, authToken, sessionID, WinShmProfileHybrid)
	if err != nil {
		t.Fatalf("WinShmClientAttach failed: %v", err)
	}

	results := make(chan error, 1)
	go func() {
		buf := make([]byte, 128)
		_, err := server.WinShmReceive(buf, 1000)
		results <- err
	}()

	time.Sleep(20 * time.Millisecond)
	client.WinShmClose()

	select {
	case err := <-results:
		if !errors.Is(err, ErrWinShmDisconnected) {
			t.Fatalf("WinShmReceive after peer close = %v, want ErrWinShmDisconnected", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WinShmReceive did not observe peer close")
	}
}

func TestWinShmReceiveTimeoutHybrid(t *testing.T) {
	runDir := t.TempDir()
	service := "timeout-hybrid"
	const authToken uint64 = 0x8912
	const sessionID uint64 = 17

	server, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096)
	if err != nil {
		t.Fatalf("WinShmServerCreate failed: %v", err)
	}
	defer server.WinShmDestroy()

	client, err := WinShmClientAttach(runDir, service, authToken, sessionID, WinShmProfileHybrid)
	if err != nil {
		t.Fatalf("WinShmClientAttach failed: %v", err)
	}
	defer client.WinShmClose()

	buf := make([]byte, 128)
	if _, err := server.WinShmReceive(buf, 10); !errors.Is(err, ErrWinShmTimeout) {
		t.Fatalf("WinShmReceive hybrid timeout = %v, want %v", err, ErrWinShmTimeout)
	}
}

func TestWinShmReceiveTimeoutBusywait(t *testing.T) {
	runDir := t.TempDir()
	service := "timeout-busywait"
	const authToken uint64 = 0x8913
	const sessionID uint64 = 19

	server, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileBusywait, 4096, 4096)
	if err != nil {
		t.Fatalf("WinShmServerCreate failed: %v", err)
	}
	defer server.WinShmDestroy()

	client, err := WinShmClientAttach(runDir, service, authToken, sessionID, WinShmProfileBusywait)
	if err != nil {
		t.Fatalf("WinShmClientAttach failed: %v", err)
	}
	defer client.WinShmClose()

	buf := make([]byte, 128)
	if _, err := server.WinShmReceive(buf, 10); !errors.Is(err, ErrWinShmTimeout) {
		t.Fatalf("WinShmReceive busywait timeout = %v, want %v", err, ErrWinShmTimeout)
	}
}

func TestWinShmReceiveHybridImmediateReadyAfterSpinSkipped(t *testing.T) {
	runDir := t.TempDir()
	service := "hybrid-immediate-ready"
	const authToken uint64 = 0x8915
	const sessionID uint64 = 25

	server, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096)
	if err != nil {
		t.Fatalf("WinShmServerCreate failed: %v", err)
	}
	defer server.WinShmDestroy()

	client, err := WinShmClientAttach(runDir, service, authToken, sessionID, WinShmProfileHybrid)
	if err != nil {
		t.Fatalf("WinShmClientAttach failed: %v", err)
	}
	defer client.WinShmClose()

	client.SpinTries = 0

	msg := []byte("hybrid-ready")
	if err := server.WinShmSend(msg); err != nil {
		t.Fatalf("WinShmSend failed: %v", err)
	}

	buf := make([]byte, 8192)
	n, err := client.WinShmReceive(buf, 1000)
	if err != nil {
		t.Fatalf("WinShmReceive failed: %v", err)
	}
	if got := string(buf[:n]); got != string(msg) {
		t.Fatalf("payload = %q, want %q", got, string(msg))
	}
}

func TestWinShmReceiveBusywaitImmediateReadyAfterSpinSkipped(t *testing.T) {
	runDir := t.TempDir()
	service := "busywait-immediate-ready"
	const authToken uint64 = 0x8916
	const sessionID uint64 = 27

	server, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileBusywait, 4096, 4096)
	if err != nil {
		t.Fatalf("WinShmServerCreate failed: %v", err)
	}
	defer server.WinShmDestroy()

	client, err := WinShmClientAttach(runDir, service, authToken, sessionID, WinShmProfileBusywait)
	if err != nil {
		t.Fatalf("WinShmClientAttach failed: %v", err)
	}
	defer client.WinShmClose()

	client.SpinTries = 0

	msg := []byte("busywait-ready")
	if err := server.WinShmSend(msg); err != nil {
		t.Fatalf("WinShmSend failed: %v", err)
	}

	buf := make([]byte, 8192)
	n, err := client.WinShmReceive(buf, 1000)
	if err != nil {
		t.Fatalf("WinShmReceive failed: %v", err)
	}
	if got := string(buf[:n]); got != string(msg) {
		t.Fatalf("payload = %q, want %q", got, string(msg))
	}
}

func TestWinShmReceiveBusywaitDetectsPeerClosed(t *testing.T) {
	runDir := t.TempDir()
	service := "busywait-peer-closed"
	const authToken uint64 = 0x8914
	const sessionID uint64 = 21

	server, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileBusywait, 4096, 4096)
	if err != nil {
		t.Fatalf("WinShmServerCreate failed: %v", err)
	}
	defer server.WinShmDestroy()

	client, err := WinShmClientAttach(runDir, service, authToken, sessionID, WinShmProfileBusywait)
	if err != nil {
		t.Fatalf("WinShmClientAttach failed: %v", err)
	}

	results := make(chan error, 1)
	go func() {
		buf := make([]byte, 128)
		_, err := server.WinShmReceive(buf, 1000)
		results <- err
	}()

	time.Sleep(20 * time.Millisecond)
	client.WinShmClose()

	select {
	case err := <-results:
		if !errors.Is(err, ErrWinShmDisconnected) {
			t.Fatalf("WinShmReceive busywait after peer close = %v, want ErrWinShmDisconnected", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WinShmReceive busywait did not observe peer close")
	}
}

func TestWinShmReceiveNullContext(t *testing.T) {
	var ctx WinShmContext
	buf := make([]byte, 16)
	if _, err := ctx.WinShmReceive(buf, 1); !errors.Is(err, ErrWinShmBadParam) {
		t.Fatalf("WinShmReceive on null context = %v, want ErrWinShmBadParam", err)
	}
}

func TestWinShmReceiveIgnoresSpuriousWakeClient(t *testing.T) {
	testWinShmReceiveIgnoresSpuriousWake(t, false)
}

func TestWinShmReceiveIgnoresSpuriousWakeServer(t *testing.T) {
	testWinShmReceiveIgnoresSpuriousWake(t, true)
}

func testWinShmReceiveIgnoresSpuriousWake(t *testing.T, serverReceives bool) {
	t.Helper()

	runDir := t.TempDir()
	service := "spurious-wake"
	const authToken uint64 = 0x1234
	const sessionID uint64 = 7

	server, err := WinShmServerCreate(runDir, service, authToken, sessionID, WinShmProfileHybrid, 4096, 4096)
	if err != nil {
		t.Fatalf("WinShmServerCreate failed: %v", err)
	}
	defer server.WinShmDestroy()

	client, err := WinShmClientAttach(runDir, service, authToken, sessionID, WinShmProfileHybrid)
	if err != nil {
		t.Fatalf("WinShmClientAttach failed: %v", err)
	}
	defer client.WinShmClose()

	first := []byte("first-message")
	second := []byte("second-message")

	var sender *WinShmContext
	var receiver *WinShmContext
	var waitingOff int
	var waitEvent syscall.Handle

	if serverReceives {
		sender = client
		receiver = server
		waitingOff = wshOFFReqServerWaiting
		waitEvent = server.reqEvent
	} else {
		sender = server
		receiver = client
		waitingOff = wshOFFRespClientWaiting
		waitEvent = client.respEvent
	}

	if err := sender.WinShmSend(first); err != nil {
		t.Fatalf("first WinShmSend failed: %v", err)
	}

	firstBuf := make([]byte, 128)
	firstLen, err := receiver.WinShmReceive(firstBuf, 1000)
	if err != nil {
		t.Fatalf("first WinShmReceive failed: %v", err)
	}
	if got := string(firstBuf[:firstLen]); got != string(first) {
		t.Fatalf("first payload = %q, want %q", got, string(first))
	}

	results := make(chan shmReceiveResult, 1)
	go func() {
		buf := make([]byte, 128)
		n, err := receiver.WinShmReceive(buf, 1000)
		if err != nil {
			results <- shmReceiveResult{err: err}
			return
		}
		results <- shmReceiveResult{payload: append([]byte(nil), buf[:n]...)}
	}()

	data := unsafe.Slice((*byte)(unsafe.Pointer(receiver.base)), receiver.size)
	deadline := time.Now().Add(time.Second)
	for atomic.LoadInt32((*int32)(unsafe.Pointer(&data[waitingOff]))) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("receiver never entered the wait state")
		}
		time.Sleep(time.Millisecond)
	}

	if ret, _, _ := procSetEvent.Call(uintptr(waitEvent)); ret == 0 {
		t.Fatal("SetEvent failed for spurious wake probe")
	}

	time.Sleep(10 * time.Millisecond)

	if err := sender.WinShmSend(second); err != nil {
		t.Fatalf("second WinShmSend failed: %v", err)
	}

	select {
	case res := <-results:
		if res.err != nil {
			t.Fatalf("second WinShmReceive failed: %v", res.err)
		}
		if got := string(res.payload); got != string(second) {
			t.Fatalf("second payload = %q, want %q", got, string(second))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second WinShmReceive timed out")
	}
}
