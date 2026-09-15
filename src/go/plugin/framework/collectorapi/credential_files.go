// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/credentialfile"
)

// CredentialFiles is the collector-owned file reader. Its helper starts only
// when a configured file is read. The runtime closes it after collector Cleanup.
func (b *Base) CredentialFiles() credentialfile.FileReader {
	b.filesMu.Lock()
	defer b.filesMu.Unlock()
	if b.files == nil {
		b.files = credentialfile.New()
		if b.filesClosed {
			_ = b.files.Close()
		}
	}
	return b.files
}

// SetCredentialFiles transfers ownership of an explicit reader to the collector.
// Configure dependencies before Init; replacing a reader closes the previous one.
func (b *Base) SetCredentialFiles(files credentialfile.FileReader) {
	b.filesMu.Lock()
	previous := b.files
	b.files = files
	closed := b.filesClosed
	b.filesMu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	if closed && files != nil {
		_ = files.Close()
	}
}

func (b *Base) closeCredentialFiles() {
	b.filesMu.Lock()
	if b.filesClosed {
		b.filesMu.Unlock()
		return
	}
	b.filesClosed = true
	files := b.files
	b.filesMu.Unlock()
	if files != nil {
		_ = files.Close()
	}
}

// CleanupCollector releases collector and framework-owned resources. Deferring
// Base cleanup preserves ownership even if the collector's cleanup panics.
func CleanupCollector(ctx context.Context, module interface {
	GetBase() *Base
	Cleanup(context.Context)
}) {
	if base := module.GetBase(); base != nil {
		defer base.closeCredentialFiles()
	}
	module.Cleanup(ctx)
}
