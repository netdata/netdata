//go:build windows

package raw

// Robustness reproduction tests (SOW-0040), Windows.
//
// Each test asserts the correct behavior and runs every blocking step in a
// goroutine under a watchdog, and every later wait on such a goroutine is
// bounded too (recvWithin), so a regression fails instead of hanging.
// Bounds are deliberately lenient so they do not dictate a fix design.

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

const (
	winRobustHelperEnv           = "NETIPC_ROBUSTNESS_HELPER"
	winFileFlagFirstPipeInstance = 0x00080000
)

func winWithin(limit time.Duration, f func()) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
		return true
	case <-time.After(limit):
		return false
	}
}

func winSlowIncrementDispatch(delay time.Duration) DispatchHandler {
	return IncrementDispatch(func(v uint64) (uint64, bool) {
		time.Sleep(delay)
		return v + 1, true
	})
}

type winRobustServer struct {
	server *Server
	doneCh chan struct{}
}

func startWinRobustServer(service string, cfg windows.ServerConfig, method uint16, handler DispatchHandler, workers int) *winRobustServer {
	s := NewServerWithWorkers(winTestRunDir, service, cfg, method, handler, workers)
	return &winRobustServer{server: s, doneCh: runServerForTest(s)}
}

// stopWithin stops the server and reports whether Run returned in time.
func (rs *winRobustServer) stopWithin(limit time.Duration) bool {
	stopped := make(chan struct{})
	go func() {
		rs.server.Stop()
		close(stopped)
	}()
	select {
	case <-rs.doneCh:
		return true
	case <-time.After(limit):
		return false
	}
}

// winServedOnce connects once and completes one increment call. Callers
// make sure a worker is free first, so any failure is a real one.
func winServedOnce(service string, cfg windows.ClientConfig) error {
	c := NewIncrementClient(winTestRunDir, service, cfg)
	defer c.Close()
	c.Refresh()
	if !c.Ready() {
		return fmt.Errorf("state %v", c.Status().State)
	}
	v, err := c.CallIncrementWithTimeout(1, 1000)
	if err != nil || v != 2 {
		return fmt.Errorf("call: v=%d err=%v", v, err)
	}
	return nil
}

// winBlockingIncrementDispatch parks every request; the helper process
// holding it is killed by the test.
func winBlockingIncrementDispatch(entered func()) DispatchHandler {
	return IncrementDispatch(func(v uint64) (uint64, bool) {
		entered()
		select {}
	})
}

// winHelperEntered reports, per helper server process, that its handler
// holds a request.
var winHelperEntered = map[*exec.Cmd]chan struct{}{}

func winOpenPipeSilently(t *testing.T, service string) syscall.Handle {
	t.Helper()
	name, err := windows.BuildPipeName(winTestRunDir, service)
	if err != nil {
		t.Fatalf("pipe name: %v", err)
	}
	// The server is listening, so one open must succeed.
	h, err := syscall.CreateFile(&name[0], syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatalf("could not open pipe for %s: %v", service, err)
	}
	return h
}

func winSpawnHelper(t *testing.T, mode, service string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestWinRobustnessHelperProcess$", "-test.v")
	cmd.Env = append(os.Environ(), winRobustHelperEnv+"="+mode+":"+service)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	ready := make(chan bool, 1)
	entered := make(chan struct{}, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if strings.Contains(sc.Text(), "HELPER_READY") {
				ready <- true
			}
			if strings.Contains(sc.Text(), "HELPER_HANDLER_ENTERED") {
				entered <- struct{}{}
			}
		}
		close(ready)
	}()
	winHelperEntered[cmd] = entered
	select {
	case ok := <-ready:
		if !ok {
			t.Fatalf("helper %s exited before ready", mode)
		}
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("helper %s did not become ready", mode)
	}
	return cmd
}

// TestWinRobustnessHelperProcess is the child-process entry point; inert
// unless spawned by winSpawnHelper.
func TestWinRobustnessHelperProcess(t *testing.T) {
	spec := os.Getenv(winRobustHelperEnv)
	if spec == "" {
		return
	}
	mode, service, _ := strings.Cut(spec, ":")
	switch mode {
	case "shm_client":
		// The server listens before the helper starts: one connect.
		c := NewIncrementClient(winTestRunDir, service, testWinShmClientConfig())
		c.Refresh()
		if v, err := c.CallIncrementWithTimeout(1, 2000); err != nil || v != 2 {
			fmt.Printf("HELPER_FAILED v=%d err=%v\n", v, err)
			os.Exit(1)
		}
	case "shm_server":
		startWinRobustServer(service, testWinShmServerConfig(), protocol.MethodIncrement,
			winBlockingIncrementDispatch(func() { fmt.Println("HELPER_HANDLER_ENTERED") }), 2)
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
	fmt.Println("HELPER_READY")
	select {} // until the test kills this process
}

// K2: SHM server must release a session once its client crashes.
func TestWinRobustShmSessionReleasedAfterClientCrash(t *testing.T) {
	svc := uniqueWinService("go_wrob_shm_crash")
	rs := startWinRobustServer(svc, testWinShmServerConfig(), protocol.MethodIncrement, winSlowIncrementDispatch(0), 2)

	for i := 0; i < 2; i++ {
		cmd := winSpawnHelper(t, "shm_client", svc)
		_ = cmd.Process.Kill() // TerminateProcess
		_ = cmd.Wait()
	}
	// Both workers must notice their crashed clients and free the slots.
	waitSessionExits(rs.server, 2)
	served := winServedOnce(svc, testWinShmClientConfig())
	rs.stopWithin(5 * time.Second)
	if served != nil {
		t.Errorf("after 2 crashed SHM clients on 2 workers, a new client must be served: %v", served)
	}
}

// K3: a client that opens the pipe but never sends HELLO.
func TestWinRobustSilentClientDoesNotBlockOtherClients(t *testing.T) {
	svc := uniqueWinService("go_wrob_silent")
	rs := startWinRobustServer(svc, testWinServerConfig(), protocol.MethodIncrement, winSlowIncrementDispatch(0), 4)

	silent := winOpenPipeSilently(t, svc)
	waitHandshakesStarted(rs.server, 1)
	var served error = fmt.Errorf("did not finish")
	finished := winWithin(5*time.Second, func() {
		served = winServedOnce(svc, testWinClientConfig())
	})

	syscall.CloseHandle(silent)
	rs.stopWithin(5 * time.Second)
	if !finished || served != nil {
		t.Errorf("a legitimate client must be served within 5s while a silent client is connected (finished=%v err=%v)", finished, served)
	}
}

func TestWinRobustSilentClientDoesNotBlockShutdown(t *testing.T) {
	svc := uniqueWinService("go_wrob_silent_stop")
	rs := startWinRobustServer(svc, testWinServerConfig(), protocol.MethodIncrement, winSlowIncrementDispatch(0), 4)

	silent := winOpenPipeSilently(t, svc)
	waitHandshakesStarted(rs.server, 1)
	stopped := rs.stopWithin(3 * time.Second)

	syscall.CloseHandle(silent)
	<-rs.doneCh
	if !stopped {
		t.Error("server Run must return within 3s of Stop while a silent client is connected")
	}
}

// A silent client holds a worker only until the server handshake deadline.
func TestWinRobustSilentClientReleasedAtHandshakeDeadline(t *testing.T) {
	svc := uniqueWinService("go_wrob_silent_deadline")
	rs := startWinRobustServer(svc, testWinServerConfig(), protocol.MethodIncrement, winSlowIncrementDispatch(0), 1)

	silent := winOpenPipeSilently(t, svc)
	defer syscall.CloseHandle(silent)
	start := time.Now()
	waitHandshakesStarted(rs.server, 1)
	waitSessionExits(rs.server, 1)
	elapsed := time.Since(start)
	served := winServedOnce(svc, testWinClientConfig())
	deadline := time.Duration(rs.server.timings.handshakeMs) * time.Millisecond
	if elapsed < deadline {
		t.Errorf("silent client released after %v, before the %v handshake deadline", elapsed, deadline)
	}

	rs.stopWithin(5 * time.Second)
	if served != nil {
		t.Errorf("the only worker must be freed once the silent client misses the handshake deadline: %v (after %v)", served, elapsed)
	}
}

// K4: a client call must be bounded even when the server never answers HELLO.
// winFakeServerPipe creates a pipe that accepts clients but never answers
// HELLO.
func winFakeServerPipe(t *testing.T, service string) syscall.Handle {
	t.Helper()
	name, err := windows.BuildPipeName(winTestRunDir, service)
	if err != nil {
		t.Fatalf("pipe name: %v", err)
	}
	h, _, callErr := rawCreateNamedPipeW.Call(
		uintptr(unsafe.Pointer(&name[0])),
		uintptr(rawPipeAccessDuplex|winFileFlagFirstPipeInstance),
		uintptr(rawPipeTypeMessage|rawPipeReadModeMsg|rawPipeWait),
		4, rawPipeBufSize, rawPipeBufSize, 0, 0,
	)
	if syscall.Handle(h) == syscall.InvalidHandle {
		t.Fatalf("CreateNamedPipeW: %v", callErr)
	}
	return syscall.Handle(h)
}

func TestWinRobustClientBoundedWhenServerNeverSendsHelloAck(t *testing.T) {
	svc := uniqueWinService("go_wrob_nohelloack")
	h := winFakeServerPipe(t, svc)

	done := make(chan struct{})
	finished := winWithin(6*time.Second, func() {
		defer close(done)
		c := NewIncrementClient(winTestRunDir, svc, testWinClientConfig())
		c.SetCallTimeout(1000)
		c.Refresh()
		_, _ = c.CallIncrementWithTimeout(1, 1000)
		c.Close()
	})

	syscall.CloseHandle(h)
	recvWithin(t, done, "done")
	if !finished {
		t.Error("Refresh + call with a 1s timeout must return within 6s when the server never sends HELLO_ACK")
	}
}

func TestWinRobustCallReconnectBoundedByCallDeadline(t *testing.T) {
	svc := uniqueWinService("go_wrob_reconnect_noack")
	rs := startWinRobustServer(svc, testWinServerConfig(), protocol.MethodIncrement, winSlowIncrementDispatch(0), 1)

	c := NewIncrementClient(winTestRunDir, svc, testWinClientConfig())
	c.Refresh()
	if !c.Ready() {
		t.Fatalf("client not ready: state %v", c.Status().State)
	}
	if !rs.stopWithin(5 * time.Second) {
		t.Fatal("server did not stop")
	}

	// The call fails on the dead session and reconnects to a server that
	// accepts but never sends HELLO_ACK.
	h := winFakeServerPipe(t, svc)
	var err error
	var elapsed time.Duration
	done := make(chan struct{})
	finished := winWithin(6*time.Second, func() {
		defer close(done)
		start := time.Now()
		_, err = c.CallIncrementWithTimeout(1, 1000)
		elapsed = time.Since(start)
		c.Close()
	})

	syscall.CloseHandle(h)
	recvWithin(t, done, "done")
	if !finished {
		t.Fatal("a call with a 1s timeout must return within 6s")
	}
	if !errors.Is(err, protocol.ErrTimeout) {
		t.Errorf("call error = %v after %v, want ErrTimeout", err, elapsed)
	}
}

func TestWinRobustAbortEndsRefreshWaitingForHelloAck(t *testing.T) {
	svc := uniqueWinService("go_wrob_abort_refresh")
	h := winFakeServerPipe(t, svc)

	c := NewIncrementClient(winTestRunDir, svc, testWinClientConfig())
	// Only an abort can end the wait early.
	c.SetCallTimeout(10000)
	ready := make(chan bool, 1)
	go func() {
		c.Refresh()
		ready <- c.Ready()
	}()
	if err := connectRawNamedPipe(h); err != nil {
		if errno, ok := err.(syscall.Errno); !ok || errno != rawErrorPipeConnected {
			syscall.CloseHandle(h)
			t.Fatalf("client never connected to the fake server: %v", err)
		}
	}
	c.Abort()
	var got, returned bool
	select {
	case got = <-ready:
		returned = true
	case <-time.After(2 * time.Second):
	}

	syscall.CloseHandle(h)
	if !returned {
		recvWithin(t, ready, "ready")
		t.Fatal("refresh must return within 2s of the abort")
	}
	c.Close()
	if got {
		t.Error("refresh after abort reported ready")
	}
}

// N1 guard: the configured call timeout must be honored on the pipe path.
func TestWinRobustPipeCallHonorsTimeoutLongerThan100ms(t *testing.T) {
	svc := uniqueWinService("go_wrob_slow_handler")
	rs := startWinRobustServer(svc, testWinServerConfig(), protocol.MethodIncrement, winSlowIncrementDispatch(300*time.Millisecond), 2)
	defer rs.stopWithin(5 * time.Second)

	c := NewIncrementClient(winTestRunDir, svc, testWinClientConfig())
	defer c.Close()
	c.Refresh()
	start := time.Now()
	v, err := c.CallIncrementWithTimeout(41, 2000)
	if err != nil || v != 42 {
		t.Errorf("a 300ms handler must succeed with a 2000ms timeout: v=%d err=%v after %v", v, err, time.Since(start))
	}
}

// N2: shutdown must not wait forever on a worker blocked writing a response
// to a client that stopped reading.
func TestWinRobustShutdownNotBlockedByNonReadingClient(t *testing.T) {
	const big = 8 * 1024 * 1024
	svc := uniqueWinService("go_wrob_noread")
	scfg := testWinServerConfig()
	scfg.MaxRequestPayloadBytes = big
	scfg.MaxResponsePayloadBytes = big
	// After the handler returns, the worker can only be writing the 4 MiB
	// response, which blocks because the client never reads.
	handled := make(chan struct{}, 1)
	reverse := winStringReverseDispatchHandler()
	rs := startWinRobustServer(svc, scfg, protocol.MethodStringReverse, func(req, resp []byte) (int, error) {
		n, err := reverse(req, resp)
		handled <- struct{}{}
		return n, err
	}, 2)

	ccfg := testWinClientConfig()
	ccfg.MaxRequestPayloadBytes = big
	ccfg.MaxResponsePayloadBytes = big
	session, err := windows.Connect(winTestRunDir, svc, &ccfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	text := strings.Repeat("x", 4*1024*1024)
	payload := make([]byte, protocol.StringReverseHdrSize+len(text)+1)
	n := protocol.StringReverseEncode(text, payload)
	hdr := protocol.Header{Kind: protocol.KindRequest, Code: protocol.MethodStringReverse, ItemCount: 1, MessageID: 1}
	if err := session.Send(&hdr, payload[:n]); err != nil {
		t.Fatalf("send: %v", err)
	}
	waitClosed(handled, "string reverse handler")

	stopped := rs.stopWithin(3 * time.Second)
	session.Close()
	<-rs.doneCh
	if !stopped {
		t.Error("server Run must return within 3s of Stop while a worker is blocked writing")
	}
}

// N3: Stop issued before Run must not be lost.
func TestWinRobustStopBeforeRunIsHonored(t *testing.T) {
	svc := uniqueWinService("go_wrob_stop_first")
	s := NewServerWithWorkers(winTestRunDir, svc, testWinServerConfig(), protocol.MethodIncrement, winSlowIncrementDispatch(0), 2)
	s.Stop()
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		_ = s.Run()
	}()

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Error("Run after Stop must return promptly instead of serving forever")
		s.Stop()
		recvWithin(t, doneCh, "doneCh")
	}
}

// N5: an SHM client must notice a crashed server instead of waiting out its
// whole timeout.
func TestWinRobustShmClientNoticesCrashedServer(t *testing.T) {
	svc := uniqueWinService("go_wrob_dead_server")
	cmd := winSpawnHelper(t, "shm_server", svc)

	c := NewIncrementClient(winTestRunDir, svc, testWinShmClientConfig())
	defer c.Close()
	waitWinClientReady(t, c)

	type outcome struct {
		err     error
		elapsed time.Duration
	}
	result := make(chan outcome, 1)
	go func() {
		start := time.Now()
		_, err := c.CallIncrementWithTimeout(1, 10000)
		result <- outcome{err, time.Since(start)}
	}()
	waitClosed(winHelperEntered[cmd], "helper server handler") // request parked in the stalled handler
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	r := recvWithin(t, result, "result")
	if r.err == nil || r.elapsed > 3500*time.Millisecond {
		t.Errorf("call must fail within 3s of the server crash (timeout is 10s): err=%v after %v", r.err, r.elapsed)
	}
	if !errors.Is(r.err, protocol.ErrDisconnected) {
		t.Errorf("call must report the server as disconnected: err=%v", r.err)
	}
}

// A server at worker capacity completes the handshake, then closes the
// session and destroys the prepared WinSHM objects. The client must not spend
// its attach retry budget (5 s) on objects that will never appear, and must
// keep WinSHM enabled for its next connect.
func TestWinRobustShmAttachStopsWhenServerClosesSession(t *testing.T) {
	svc := uniqueWinService("go_wrob_attach_refused")
	rs := startWinRobustServer(svc, testWinShmServerConfig(), protocol.MethodIncrement, winSlowIncrementDispatch(0), 1)

	holder := NewIncrementClient(winTestRunDir, svc, testWinShmClientConfig())
	holder.Refresh()
	holderReady := holder.Ready()

	refused := NewIncrementClient(winTestRunDir, svc, testWinShmClientConfig())
	defer refused.Close()
	start := time.Now()
	refused.Refresh()
	elapsed := time.Since(start)

	holder.Close()
	waitSessionExits(rs.server, 1)
	refused.Refresh()
	ready, usesShm := refused.Ready(), refused.shm != nil
	refused.Close()
	rs.stopWithin(5 * time.Second)

	if !holderReady {
		t.Error("first client must hold the only worker")
	}
	if elapsed >= time.Second {
		t.Errorf("refused connect must return within 1s (attach retry budget is 5s): %v", elapsed)
	}
	if !ready {
		t.Error("refused client must connect once the worker is free")
	}
	if !usesShm {
		t.Error("refused client must still use WinSHM")
	}
}

// A worker whose thread handle cannot be opened could not be cancelled at
// stop, so the server refuses its connection before HELLO_ACK and keeps
// serving the next one.
func TestWinRobustWorkerWithoutThreadHandleRefused(t *testing.T) {
	// Installed before the server starts, restored after it stopped.
	var failing atomic.Bool
	failing.Store(true)
	open := openWorkerThread
	openWorkerThread = func(tid uintptr) syscall.Handle {
		if failing.Load() {
			return 0
		}
		return open(tid)
	}
	defer func() { openWorkerThread = open }()

	svc := uniqueWinService("go_win_rob_nothread")
	ts := startTestServerWin(svc, protocol.MethodIncrement, winPingPongIncrementDispatch())
	defer ts.stop()

	refused := NewIncrementClient(winTestRunDir, svc, testWinClientConfig())
	refused.Refresh()
	ready := refused.Ready()
	refused.Close()
	if ready {
		t.Fatal("client became ready although its worker had no thread handle")
	}

	failing.Store(false)
	c := NewIncrementClient(winTestRunDir, svc, testWinClientConfig())
	defer c.Close()
	c.Refresh()
	if v, err := c.CallIncrement(1); err != nil || v != 2 {
		t.Fatalf("call after the refused connection = %d, %v", v, err)
	}
}

const (
	winStalledReaderRequestBytes = 8 * 1024 * 1024
	winStalledReaderPids         = 1024 * 1024
)

// startWinStalledReader completes the handshake and never reads. It closes
// started once the request's first bytes arrived or its wait ended, sets
// requestSeen before that when bytes did arrive, then holds the session until
// release is closed. The caller asserts requestSeen only when the request must
// have started: a call deadline can expire while the request is still being
// encoded, before any byte is written.
func startWinStalledReader(t *testing.T, svc string, started, release chan struct{}, requestSeen *bool) chan struct{} {
	t.Helper()
	cfg := testWinServerConfig()
	cfg.MaxRequestPayloadBytes = winStalledReaderRequestBytes
	listener, err := windows.Listen(winTestRunDir, svc, cfg)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer listener.Close()
		session, err := listener.Accept()
		if err != nil {
			close(started)
			return
		}
		defer session.Close()
		readable, err := session.WaitReadable(10_000)
		*requestSeen = err == nil && readable
		close(started)
		select {
		case <-release:
		case <-time.After(10 * time.Second):
		}
	}()
	return done
}

// A request the server never reads ends at the call deadline, or on abort,
// instead of blocking in the write.
func TestWinRobustSendBoundedByDeadlineAndAbort(t *testing.T) {
	pids := make([]uint32, winStalledReaderPids)
	for i := range pids {
		pids[i] = uint32(i + 1) // #nosec G115 -- bounded by winStalledReaderPids.
	}
	for _, useAbort := range []bool{false, true} {
		name := "send_deadline"
		if useAbort {
			name = "send_abort"
		}
		t.Run(name, func(t *testing.T) {
			svc := uniqueWinService("go_winrob_" + name)
			started := make(chan struct{})
			release := make(chan struct{})
			requestSeen := false
			done := startWinStalledReader(t, svc, started, release, &requestSeen)

			ccfg := testWinClientConfig()
			ccfg.MaxRequestPayloadBytes = winStalledReaderRequestBytes
			client := NewAppsLookupClient(winTestRunDir, svc, ccfg)
			client.SetLookupLogicalConfig(LookupLogicalConfig{MaxItems: winStalledReaderPids})
			waitWinClientReady(t, client)

			timeoutMs := uint32(200)
			if useAbort {
				timeoutMs = 30_000
			}
			start := time.Now()
			result := make(chan error, 1)
			go func() {
				_, err := client.CallAppsLookupWithTimeout(pids, timeoutMs)
				result <- err
			}()
			if useAbort {
				recvWithin(t, started, "started")
				if !requestSeen {
					t.Error("no request bytes arrived before the abort")
				}
				client.Abort()
			}
			err := recvWithin(t, result, "result")
			elapsed := time.Since(start)
			// Closing the client first wakes a reader still waiting for bytes.
			client.Close()
			close(release)
			recvWithin(t, done, "done")

			want := protocol.ErrTimeout
			if useAbort {
				want = protocol.ErrAborted
			}
			if !errors.Is(err, want) {
				t.Fatalf("blocked send = %v, want %v", err, want)
			}
			if elapsed > 5*time.Second {
				t.Fatalf("blocked send took %v", elapsed)
			}
		})
	}
}
