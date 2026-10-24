// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/streamexec"
)

// tegrastatsInterval is the sampling interval requested from tegrastats, its documented default.
const tegrastatsInterval = time.Second

type tegrastatsMode uint8

const (
	tegrastatsUnselected tegrastatsMode = iota
	tegrastatsPrivileged
	tegrastatsUnprivileged
)

// Discovery checks presence only; ndsudo validates the candidate and reports unsafe installations.
func fixedTegrastatsExists() bool {
	_, err := os.Lstat("/usr/bin/tegrastats")
	return err == nil
}

func lookupTegrastats() (string, error) {
	path, err := exec.LookPath("tegrastats")
	if err != nil {
		return "", errors.New("tegrastats executable not found in PATH")
	}
	return path, nil
}

// newTegrastatsSource preserves the job's selected route across source restarts.
// A fixed-path candidate can start privileged collection even when PATH lookup failed.
func (c *Collector) newTegrastatsSource(path string, lookupErr error) (*streamexec.Source[sample], error) {
	return streamexec.New(streamexec.Config[sample]{
		Name: "tegrastats",
		Start: func(ctx context.Context, stdout *os.File) (*ndexec.Process, error) {
			return c.startTegrastats(ctx, ndexec.ProcessOptions{Stdout: stdout}, path, lookupErr)
		},
		NewDecoder: func() streamexec.Decoder[sample] { return streamexec.DecoderFunc[sample](decodeRecord) },
		Timing:     c.timing,
		Logger:     c.Logger,
	})
}

func (c *Collector) startTegrastats(ctx context.Context, opts ndexec.ProcessOptions, path string, lookupErr error) (*ndexec.Process, error) {
	if c.mode != tegrastatsUnprivileged {
		process, err := c.acquisition.StartNDSudo(ctx, opts, "tegrastats")
		if err == nil {
			c.mode = tegrastatsPrivileged
			return process, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Never change routes after a privileged producer has successfully started.
		if c.mode == tegrastatsPrivileged || !errors.Is(err, ndexec.ErrSupervisedUnavailable) {
			return nil, err
		}
		c.mode = tegrastatsUnprivileged
		c.Warningf("restricted privileged tegrastats unavailable (%v); selected unprivileged collection; GPU clocks and EMC readings may be missing", err)
	}
	if lookupErr != nil {
		return nil, fmt.Errorf("unprivileged tegrastats unavailable: %w", lookupErr)
	}
	return c.acquisition.StartUnprivileged(ctx, opts, path, "--interval", strconv.FormatInt(tegrastatsInterval.Milliseconds(), 10))
}

func decodeRecord(line []byte) (sample, bool) { return parseRecord(string(line)) }
