// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package safefile

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestRegularFileReadersRejectFIFOWithoutBlocking(t *testing.T) {
	for name, read := range map[string]func(string) error{
		"Read": func(path string) error { _, err := Read(path); return err },
		"Open": func(path string) error {
			file, err := Open(path)
			if file != nil {
				file.Close()
			}
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "value")
			require.NoError(t, unix.Mkfifo(path, 0o600))

			done := make(chan error, 1)
			go func() {
				done <- read(path)
			}()

			select {
			case err := <-done:
				require.ErrorIs(t, err, ErrFile)
				require.ErrorIs(t, err, ErrNotRegular)
			case <-time.After(time.Second):
				t.Fatal("reading a FIFO blocked")
			}
		})
	}
}
