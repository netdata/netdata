// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/streamexec"
)

// tegrastatsInterval is the sampling interval requested from tegrastats, its documented default.
const tegrastatsInterval = time.Second

func lookupTegrastats() (string, error) {
	path, err := exec.LookPath("tegrastats")
	if err != nil {
		return "", errors.New("tegrastats executable not found in PATH")
	}
	return path, nil
}

// newTegrastatsSource supervises tegrastats at path, started through the unprivileged command helper.
func (c *Collector) newTegrastatsSource(path string) (*streamexec.Source[sample], error) {
	interval := strconv.FormatInt(tegrastatsInterval.Milliseconds(), 10)
	return streamexec.New(streamexec.Config[sample]{
		Name: "tegrastats",
		Start: func(ctx context.Context, stdout *os.File) (*ndexec.Process, error) {
			return ndexec.StartUnprivilegedProcess(
				ctx,
				ndexec.ProcessOptions{
					Stdout: stdout,
				},
				path,
				"--interval",
				interval,
			)
		},
		NewDecoder: func() streamexec.Decoder[sample] { return streamexec.DecoderFunc[sample](decodeRecord) },
		Timing:     c.timing,
		Logger:     c.Logger,
	})
}

func decodeRecord(line []byte) (sample, bool) { return parseRecord(string(line)) }
