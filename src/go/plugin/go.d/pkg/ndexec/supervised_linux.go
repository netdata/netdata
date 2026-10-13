// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const supervisedStartupTimeout = 5 * time.Second
const supervisedProtocolLimit = 128

type supervisedProcess struct {
	cmd              *exec.Cmd
	observer         exitObserver
	control, status  *os.File
	lease            *acquisitionLease
	startup          chan string
	protocolDone     chan struct{}
	data             string
	readErr, waitErr error
}

func startSupervisedNDSudo(ctx context.Context, opts ProcessOptions, helper, command string, lease *acquisitionLease) (*Process, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	started := false
	defer func() {
		if !started {
			lease.release()
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	controlRead, controlWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer controlRead.Close()
	statusRead, statusWrite, err := os.Pipe()
	if err != nil {
		controlWrite.Close()
		return nil, err
	}
	defer statusWrite.Close()
	defer func() {
		if !started {
			controlWrite.Close()
			statusRead.Close()
		}
	}()
	if opts.Stdin == nil || opts.Stdout == nil || opts.Stderr == nil {
		null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
		defer null.Close()
		if opts.Stdin == nil {
			opts.Stdin = null
		}
		if opts.Stdout == nil {
			opts.Stdout = null
		}
		if opts.Stderr == nil {
			opts.Stderr = null
		}
	}
	observer, err := newExitObserver()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(helper, command)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = opts.Stdin, opts.Stdout, opts.Stderr
	cmd.ExtraFiles = []*os.File{controlRead, statusWrite}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = observer.close()
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
			return nil, errors.Join(ErrSupervisedUnavailable, err)
		}
		return nil, fmt.Errorf("start restricted ndsudo: %w", err)
	}
	started = true
	// Close our inherited-end copies before starting any protocol reader/waiter.
	_ = controlRead.Close()
	_ = statusWrite.Close()
	h := &supervisedProcess{cmd: cmd, observer: observer, control: controlWrite, status: statusRead,
		lease: lease, startup: make(chan string, 1), protocolDone: make(chan struct{})}
	go h.readProtocol()
	p := ownProcess(ctx, h)
	timer := time.NewTimer(supervisedStartupTimeout)
	defer timer.Stop()
	abort := func(err error) (*Process, error) { _ = p.stop(); return nil, err }
	select {
	case first := <-h.startup:
		if err := ctx.Err(); err != nil {
			return abort(err)
		}
		if first == "NDSUP1 started\n" {
			return p, nil
		}
		if !knownSupervisedRefusal(first) && first != "NDSUP1 failed\n" {
			return abort(errors.New("invalid restricted ndsudo startup protocol"))
		}
		// Safe fallback is returned only after helper exit and full private evidence.
		select {
		case <-p.done:
			err := p.Wait()
			if err == nil {
				err = errors.New("restricted ndsudo failed before exec acknowledgement")
			}
			return nil, err
		case <-ctx.Done():
			return abort(ctx.Err())
		case <-timer.C:
			return abort(errors.New("restricted ndsudo startup cleanup timed out"))
		}
	case <-ctx.Done():
		return abort(ctx.Err())
	case <-timer.C:
		return abort(errors.New("restricted ndsudo exec acknowledgement timed out"))
	}
}

func knownSupervisedRefusal(first string) bool {
	switch first {
	case "NDSUP1 refused privilege\n", "NDSUP1 refused missing\n", "NDSUP1 refused unsafe\n",
		"NDSUP1 refused arguments\n", "NDSUP1 refused setup\n":
		return true
	default:
		return false
	}
}

func (p *supervisedProcess) readProtocol() {
	defer close(p.protocolDone)
	reader := bufio.NewReader(io.LimitReader(p.status, supervisedProtocolLimit))
	first, err := reader.ReadString('\n')
	p.startup <- first
	if err != nil {
		p.data, p.readErr = first, err
		return
	}
	rest, err := io.ReadAll(reader)
	p.data, p.readErr = first+string(rest), err
	if len(p.data) >= supervisedProtocolLimit {
		p.readErr = errors.New("oversized restricted ndsudo protocol")
	}
}

func (p *supervisedProcess) observeExit() error {
	err := p.observer.wait(p.cmd.Process.Pid)
	// A mismatched helper could leak the status writer. Do not let that prevent
	// Wait from reporting unverified cleanup after the supervisor has exited.
	_ = p.status.SetReadDeadline(time.Now().Add(time.Second))
	return err
}
func (p *supervisedProcess) terminate() error { return p.control.Close() }
func (p *supervisedProcess) reap() error {
	p.waitErr = p.cmd.Wait()
	<-p.protocolDone
	return p.waitErr
}
func (p *supervisedProcess) release() error {
	defer p.status.Close()
	defer p.observer.close()
	if p.readErr != nil {
		return errors.Join(ErrSupervisedNotDrained, p.readErr)
	}
	first, rest, ok := strings.Cut(p.data, "\n")
	if !ok {
		return ErrSupervisedNotDrained
	}
	first += "\n"
	if knownSupervisedRefusal(first) && rest == "" && supervisedExitCode(p.waitErr) == 126 {
		p.lease.release()
		if first == "NDSUP1 refused privilege\n" || first == "NDSUP1 refused missing\n" {
			return ErrSupervisedUnavailable
		}
		return fmt.Errorf("restricted ndsudo rejected startup: %s", strings.TrimSpace(first))
	}
	if (first == "NDSUP1 started\n" || first == "NDSUP1 failed\n") &&
		strings.HasPrefix(rest, "NDSUP1 cleaned ") && strings.HasSuffix(rest, "\n") {
		raw := strings.TrimSuffix(strings.TrimPrefix(rest, "NDSUP1 cleaned "), "\n")
		if validCommandCompletion(raw, p.waitErr) {
			p.lease.release()
			if first == "NDSUP1 failed\n" {
				return errors.New("restricted ndsudo payload setup or exec failed")
			}
			return nil
		}
	}
	return ErrSupervisedNotDrained
}

func supervisedExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
