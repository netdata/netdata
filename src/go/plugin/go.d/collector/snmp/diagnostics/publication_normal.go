// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type NormalCheckpoint interface{ CaptureNormal() (*NormalDevice, error) }

// NormalWriter is authority for one accepted runtime, even when the framework
// reuses its configuration and registration IDs for a replacement.
type NormalWriter struct {
	publisher             *Publisher
	owner                 string
	registration, runtime uint64
	version               uint64
	pending               NormalCheckpoint
	queued                bool
}

type normalPublication struct {
	onDisk   map[uint64]uint64
	writers  map[string]*NormalWriter
	sequence uint64
	queue    []*NormalWriter
	retired  []normalRetirement
	// Run's serial writer owns activation and disk cleanup.
	activated    bool
	prunePending bool
}

func (p *Publisher) ReplaceNormal(previous, owner string, registration uint64) *NormalWriter {
	if p == nil || owner == "" {
		return nil
	}
	p.mu.Lock()
	if p.normal.writers == nil {
		p.normal.writers = make(map[string]*NormalWriter)
	}
	p.retireNormalLocked(previous)
	p.retireNormalLocked(owner)
	var writer *NormalWriter
	if registration != 0 {
		p.normal.sequence++
		writer = &NormalWriter{publisher: p, owner: owner, registration: registration, runtime: p.normal.sequence}
		p.normal.writers[owner] = writer
	}
	p.mu.Unlock()
	p.notify()
	return writer
}

func (p *Publisher) RemoveNormal(owner string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.retireNormalLocked(owner)
	p.mu.Unlock()
	p.notify()
}

func (p *Publisher) retireNormalLocked(owner string) {
	if writer := p.normal.writers[owner]; writer != nil {
		delete(p.normal.writers, owner)
		writer.pending = nil
		p.normal.retired = append(p.normal.retired, normalRetirement{registration: writer.registration, runtime: writer.runtime})
	}
}

func (w *NormalWriter) filename() string { return normalFilename(w.registration) }

func (w *NormalWriter) Update(cut NormalCheckpoint) {
	if w == nil || cut == nil {
		return
	}
	p := w.publisher
	p.mu.Lock()
	if p.normal.writers[w.owner] == w {
		w.version++
		w.pending = cut
		p.queueNormalLocked(w)
	}
	p.mu.Unlock()
	p.notify()
}

func (p *Publisher) queueNormalLocked(w *NormalWriter) {
	if !w.queued && w.pending != nil {
		w.queued = true
		p.normal.queue = append(p.normal.queue, w)
	}
}

var errNormalRetired = errors.New("normal runtime retired before publication")

func (p *Publisher) publishNormal(ctx context.Context) error {
	p.mu.Lock()
	through := len(p.normal.queue)
	p.mu.Unlock()
	var errs []error
	for range through {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.mu.Lock()
		writer := p.normal.queue[0]
		p.normal.queue[0] = nil
		p.normal.queue = p.normal.queue[1:]
		writer.queued = false
		cut, version := writer.pending, writer.version
		p.mu.Unlock()
		if cut == nil {
			continue
		}
		err := p.publishNormalCut(ctx, writer, cut)
		p.mu.Lock()
		if err == nil && writer.version == version {
			writer.pending = nil
		}
		if p.normal.writers[writer.owner] == writer {
			p.queueNormalLocked(writer)
		}
		p.mu.Unlock()
		if err != nil && !errors.Is(err, errNormalRetired) {
			errs = append(errs, err)
		}
		// The next loop holds only one source cut, never a batch of device snapshots.
	}
	if err := p.cleanupNormal(ctx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (p *Publisher) publishNormalCut(ctx context.Context, writer *NormalWriter, cut NormalCheckpoint) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("normal capture panic: %v", v)
		}
	}()
	device, err := cut.CaptureNormal()
	if err != nil {
		return err
	}
	if device == nil {
		return errors.New("normal capture returned no device")
	}
	// The provider owns its cut; publication adds identity to a private header.
	normal := *device
	normal.RegistrationID, normal.RuntimeID = writer.registration, writer.runtime
	if err := normal.Validate(); err != nil {
		return fmt.Errorf("invalid normal capture: %w", err)
	}
	document := p.document(KindNormal, Snapshot{})
	document.Normal = &normal
	path := filepath.Join(p.directory, NormalDirectory, p.runID, writer.filename())
	replace := func(from, to string) error {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.normal.writers[writer.owner] != writer {
			return errNormalRetired
		}
		if err := p.rename(from, to); err != nil {
			return err
		}
		if p.normal.onDisk == nil {
			p.normal.onDisk = make(map[uint64]uint64)
		}
		p.normal.onDisk[writer.registration] = writer.runtime
		return nil
	}
	if err := p.writeFile(ctx, path, document, replace); err != nil {
		return err
	}
	return p.activateNormal(ctx)
}

type normalRetirement struct{ registration, runtime uint64 }

func (p *Publisher) removeRetiredNormal(retired normalRetirement) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if current := p.normal.onDisk[retired.registration]; current != 0 && current != retired.runtime {
		return nil
	}
	path := filepath.Join(p.directory, NormalDirectory, p.runID, normalFilename(retired.registration))
	if err := p.remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	delete(p.normal.onDisk, retired.registration)
	return nil
}
