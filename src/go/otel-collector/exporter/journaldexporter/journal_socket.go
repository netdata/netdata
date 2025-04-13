package journaldexporter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"time"
)

const defaultJournalSocket = "/run/systemd/journal/socket"

func newSocketJournalClient() (*socketJournalClient, error) {
	if _, err := os.Stat(defaultJournalSocket); os.IsNotExist(err) {
		return nil, fmt.Errorf("journal socket does not exist: %w", err)
	}

	conn, err := createJournalConn()
	if err != nil {
		return nil, fmt.Errorf("failed to create journal connection: %w", err)
	}

	tempFile, err := createUnlinkedTempFile()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to create temporary file: %w", err)
	}

	return &socketJournalClient{
		socketPath: defaultJournalSocket,
		conn:       conn,
		done:       make(chan struct{}),
		tempFile:   tempFile,
	}, nil
}

type socketJournalClient struct {
	socketPath string
	conn       *net.UnixConn
	mu         sync.Mutex
	done       chan struct{}

	// Pre-created temporary file for large messages
	tempFile *os.File
}

func (jc *socketJournalClient) sendMessage(ctx context.Context, msg []byte) error {
	if len(msg) == 0 {
		return nil
	}
	if jc.conn == nil {
		return errors.New("journal: connection is closed")
	}

	jc.mu.Lock()
	defer jc.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-jc.done:
		return errors.New("journal: client is shut down")
	default:
	}

	socketAddr := &net.UnixAddr{
		Name: jc.socketPath,
		Net:  "unixgram",
	}

	if err := jc.setConnWriteDeadline(ctx); err != nil {
		return fmt.Errorf("journal: failed to set write deadline: %w", err)
	}

	if _, _, err := jc.conn.WriteMsgUnix(msg, nil, socketAddr); err != nil {
		if !isSocketSpaceError(err) {
			return fmt.Errorf("journal: failed to write to socket: %w", err)
		}
		return jc.sendViaFd(ctx, msg, socketAddr)
	}

	return nil
}

func (jc *socketJournalClient) sendViaFd(ctx context.Context, msg []byte, socketAddr *net.UnixAddr) error {
	if _, err := jc.tempFile.Seek(0, 0); err != nil {
		return fmt.Errorf("journal: failed to seek in temporary file: %w", err)
	}

	if err := jc.tempFile.Truncate(0); err != nil {
		return fmt.Errorf("journal: failed to truncate temporary file: %w", err)
	}

	if _, err := jc.tempFile.Write(msg); err != nil {
		return fmt.Errorf("journal: failed to write to temporary file: %w", err)
	}

	if _, err := jc.tempFile.Seek(0, 0); err != nil {
		return fmt.Errorf("journal: failed to reset file position: %w", err)
	}

	rights := syscall.UnixRights(int(jc.tempFile.Fd()))

	if err := jc.setConnWriteDeadline(ctx); err != nil {
		return fmt.Errorf("journal: failed to set write deadline: %w", err)
	}

	if _, _, err := jc.conn.WriteMsgUnix([]byte{}, rights, socketAddr); err != nil {
		return fmt.Errorf("journal: failed to send file descriptor: %w", err)
	}

	return nil
}

func (jc *socketJournalClient) shutdown(ctx context.Context) error {
	jc.mu.Lock()
	defer jc.mu.Unlock()

	select {
	case <-jc.done:
		return nil
	default:
		close(jc.done)
	}

	if jc.tempFile != nil {
		_ = jc.tempFile.Close()
		jc.tempFile = nil
	}

	if jc.conn != nil {
		_ = jc.conn.Close()
		jc.conn = nil
	}

	return nil
}

func (jc *socketJournalClient) setConnWriteDeadline(ctx context.Context) error {
	var timeout = 5 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}
	return jc.conn.SetWriteDeadline(time.Now().Add(timeout))
}

func createJournalConn() (*net.UnixConn, error) {
	autobind, err := net.ResolveUnixAddr("unixgram", "")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve unix address: %w", err)
	}

	conn, err := net.ListenUnixgram("unixgram", autobind)
	if err != nil {
		return nil, fmt.Errorf("failed to create unix datagram socket: %w", err)
	}

	return conn, nil
}

func createUnlinkedTempFile() (*os.File, error) {
	file, err := os.CreateTemp("/dev/shm/", "journal.XXXXX")
	if err != nil {
		return nil, err
	}

	// Unlink the file so it's automatically cleaned up when closed
	err = syscall.Unlink(file.Name())
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	return file, nil
}

func isSocketSpaceError(err error) bool {
	// checks whether the error is signaling an "overlarge message" condition
	var opErr *net.OpError
	var sysErr *os.SyscallError

	if !errors.As(err, &opErr) || !errors.As(opErr.Err, &sysErr) {
		return false
	}

	return errors.Is(sysErr.Err, syscall.EMSGSIZE) || errors.Is(sysErr.Err, syscall.ENOBUFS)
}
