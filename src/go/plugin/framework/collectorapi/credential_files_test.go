// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/credentialfile"
	"github.com/stretchr/testify/assert"
)

type cleanupFiles struct {
	credentialfile.FileReader
	closes atomic.Int32
}

func (f *cleanupFiles) Close() error { f.closes.Add(1); return nil }

type cleanupCollector struct {
	Base
	files             *cleanupFiles
	panic             bool
	openDuringCleanup bool
}

func (c *cleanupCollector) Cleanup(context.Context) {
	c.openDuringCleanup = c.files.closes.Load() == 0
	if c.panic {
		panic("synthetic cleanup failure")
	}
}

func TestCleanupCollectorOwnsCredentialFiles(t *testing.T) {
	for _, panics := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "panic"}[panics], func(t *testing.T) {
			files := &cleanupFiles{}
			c := &cleanupCollector{files: files, panic: panics}
			c.SetCredentialFiles(files)
			assert.Same(t, files, c.CredentialFiles())
			cleanup := func() { CleanupCollector(context.Background(), c) }
			if panics {
				assert.Panics(t, cleanup)
			} else {
				assert.NotPanics(t, cleanup)
			}
			assert.True(t, c.openDuringCleanup)
			assert.EqualValues(t, 1, files.closes.Load())
			c.closeCredentialFiles()
			assert.EqualValues(t, 1, files.closes.Load())
		})
	}
}

func TestCredentialFilesCannotRestartAfterCleanup(t *testing.T) {
	b := &Base{}
	b.closeCredentialFiles()
	_, err := b.CredentialFiles().Read(context.Background(), "unused-path")
	assert.Error(t, err)
	files := &cleanupFiles{}
	b.SetCredentialFiles(files)
	assert.EqualValues(t, 1, files.closes.Load())
}
