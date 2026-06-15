//go:build unix

package posix

import (
	"errors"
	"net"
	"os"
	"syscall"
	"time"
)

type staleResult int

const (
	staleNotExist   staleResult = 0
	staleRecovered  staleResult = 1
	staleLiveServer staleResult = 2
)

const (
	staleDialAttempts   = 3
	staleDialRetryDelay = 50 * time.Millisecond
	staleDialTimeout    = 1 * time.Second
)

func dialStaleCandidate(path string) error {
	var err error
	for attempt := 0; attempt < staleDialAttempts; attempt++ {
		var conn net.Conn
		conn, err = net.DialTimeout("unixpacket", path, staleDialTimeout)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if !errors.Is(err, syscall.ECONNREFUSED) {
			return err
		}
		if attempt+1 < staleDialAttempts {
			time.Sleep(staleDialRetryDelay)
		}
	}
	return err
}

func checkAndRecoverStale(path string) staleResult {
	_, err := os.Stat(path)
	if err != nil {
		return staleNotExist
	}

	err = dialStaleCandidate(path)
	if err == nil {
		return staleLiveServer
	}

	if errors.Is(err, syscall.ENOENT) {
		return staleNotExist
	}
	if errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE) {
		// Cannot probe liveness (fd exhaustion) — keep the endpoint rather
		// than risk deleting a live socket.
		return staleLiveServer
	}
	// Nothing accepted the connection: a dead server's socket, or a foreign
	// file squatting on the endpoint path. Reclaim it (os.Remove also
	// handles a directory at the path).
	if removeErr := os.Remove(path); removeErr != nil {
		if os.IsNotExist(removeErr) {
			return staleNotExist
		}
		return staleLiveServer
	}
	return staleRecovered
}
