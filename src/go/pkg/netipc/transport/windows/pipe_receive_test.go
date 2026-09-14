//go:build windows

package windows

import (
	"errors"
	"syscall"
	"testing"
	"time"
)

func TestReceiveClosedSession(t *testing.T) {
	session := &Session{handle: syscall.InvalidHandle}

	if _, _, err := session.Receive(make([]byte, 128)); !errors.Is(err, ErrBadParam) {
		t.Fatalf("Receive on closed session = %v, want ErrBadParam", err)
	}
	if _, _, err := session.ReceiveTimeout(make([]byte, 128), 1, nil); !errors.Is(err, ErrBadParam) {
		t.Fatalf("ReceiveTimeout on closed session = %v, want ErrBadParam", err)
	}
}

func TestReceiveWaitHelpers(t *testing.T) {
	if got := boundedReceiveWaitMs(-time.Millisecond, receiveAbortPollMs); got != 0 {
		t.Fatalf("boundedReceiveWaitMs negative = %d, want 0", got)
	}
	if got := boundedReceiveWaitMs(250*time.Millisecond, receiveAbortPollMs); got != receiveAbortPollMs {
		t.Fatalf("boundedReceiveWaitMs capped = %d, want %d", got, receiveAbortPollMs)
	}
	if got := boundedReceiveWaitMs(500*time.Microsecond, receiveAbortPollMs); got != 1 {
		t.Fatalf("boundedReceiveWaitMs sub-ms = %d, want 1", got)
	}
	if got := boundedReceiveWaitMs(1500*time.Microsecond, receiveAbortPollMs); got != 2 {
		t.Fatalf("boundedReceiveWaitMs ceil-ms = %d, want 2", got)
	}

	infinite := newReceiveWait(0, nil)
	waitMs, err := infinite.waitMs()
	if err != nil || waitMs != receiveAbortPollMs {
		t.Fatalf("infinite wait = %d/%v, want %d/nil", waitMs, err, receiveAbortPollMs)
	}

	expired := receiveWait{deadline: time.Now().Add(-time.Millisecond)}
	if _, err := expired.waitMs(); !errors.Is(err, ErrTimeout) {
		t.Fatalf("expired wait error = %v, want ErrTimeout", err)
	}

	abortCh := make(chan struct{})
	close(abortCh)
	aborted := newReceiveWait(0, abortCh)
	if _, err := aborted.waitMs(); !errors.Is(err, ErrAborted) {
		t.Fatalf("aborted wait error = %v, want ErrAborted", err)
	}

	timed := newReceiveWait(50, nil)
	if timed.infinite || timed.deadline.IsZero() {
		t.Fatalf("timed wait = %+v, want finite wait with deadline", timed)
	}
}

func TestRawRecvWithTimeoutClosedSession(t *testing.T) {
	session := &Session{handle: syscall.InvalidHandle}
	if _, err := session.rawRecvWithTimeout(make([]byte, 1), receiveWait{infinite: true}); !errors.Is(err, ErrBadParam) {
		t.Fatalf("rawRecvWithTimeout on closed session = %v, want ErrBadParam", err)
	}
	if _, err := session.rawRecvWithTimeout(make([]byte, 1), receiveWait{deadline: time.Now().Add(-time.Millisecond)}); !errors.Is(err, ErrTimeout) {
		t.Fatalf("rawRecvWithTimeout expired wait = %v, want ErrTimeout", err)
	}
}
