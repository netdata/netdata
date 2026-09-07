// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
)

// CheckpointRetention is the approved history depth, not an evidence-size limit.
const CheckpointRetention = 3
const publicationRetryEvery = time.Minute

type Source interface {
	LifecycleSource
	LifecycleRevision() uint64
	LifecycleChanges() <-chan struct{}
}

// Checkpoint retains immutable native evidence. Conversion runs only on the writer.
type Checkpoint interface {
	ID() uint64
	Capture() (Snapshot, error)
}

type TopologySource interface{ Checkpoints() []Checkpoint }

type pendingCheckpoint struct {
	sequence   uint64
	checkpoint Checkpoint
}

type Publisher struct {
	*logger.Logger
	source     Source
	directory  string
	runID      string
	mu         sync.Mutex
	topology   TopologySource
	owner      string
	revision   uint64
	acceptedID uint64
	sequence   uint64
	pending    []pendingCheckpoint
	changed    chan struct{}

	// The following state belongs exclusively to Run's serial writer.
	lifecycleWritten       bool
	lifecycleRevision      uint64
	lifecycleOwnerRevision uint64
	fileSequence           uint64
	initialized            bool
	prunePending           bool
	rename                 func(string, string) error
	remove                 func(string) error
	writeFile              func(context.Context, string, Document, func(string, string) error) error
}

func NewPublisher(source Source, varLibDir string) *Publisher {
	// A prior run may have published a complete checkpoint without finishing rotation.
	return &Publisher{Logger: logger.New(), source: source, directory: DirectoryPath(varLibDir),
		runID: uuid.NewString(), changed: make(chan struct{}, 1), prunePending: true,
		rename: os.Rename, remove: os.Remove, writeFile: writeArchiveFile}
}

// SetTopology admits only an accepted configuration and its already captured cuts.
func (p *Publisher) SetTopology(owner string, source TopologySource) {
	p.mu.Lock()
	if p.owner == owner && p.topology == source {
		p.mu.Unlock()
		return
	}
	p.owner, p.topology = owner, source
	p.revision++
	p.acceptedID = 0
	p.mu.Unlock()
	if source != nil {
		p.TopologyUpdated(source)
	}
	p.notify()
}

func (p *Publisher) RemoveTopology(owner string) {
	p.mu.Lock()
	if p.owner == owner {
		p.owner = ""
		p.topology = nil
		p.revision++
		p.acceptedID = 0
	}
	p.mu.Unlock()
	p.notify()
}

func (p *Publisher) ReleaseTopology(source TopologySource) {
	p.mu.Lock()
	if p.topology == source {
		p.owner = ""
		p.topology = nil
		p.revision++
		p.acceptedID = 0
	}
	p.mu.Unlock()
	p.notify()
}

// TopologyUpdated copies at most three references, never evidence or file data.
// Admission happens here so accepted history survives a subsequent provider release.
func (p *Publisher) TopologyUpdated(source TopologySource) {
	checkpoints := source.Checkpoints()
	p.mu.Lock()
	if p.topology == source {
		for _, checkpoint := range checkpoints {
			if checkpoint.ID() <= p.acceptedID {
				continue
			}
			p.acceptedID = checkpoint.ID()
			p.sequence++
			if len(p.pending) == CheckpointRetention {
				copy(p.pending, p.pending[1:])
				p.pending = p.pending[:len(p.pending)-1]
			}
			p.pending = append(p.pending, pendingCheckpoint{sequence: p.sequence, checkpoint: checkpoint})
		}
	}
	p.mu.Unlock()
	p.notify()
}

func (p *Publisher) notify() {
	select {
	case p.changed <- struct{}{}:
	default:
	}
}

func (p *Publisher) Run(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	p.flush(ctx)
	retry := time.NewTicker(publicationRetryEvery)
	defer retry.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.changed:
		case <-p.source.LifecycleChanges():
		case <-retry.C: // Retry dirty files only; unchanged state does not serialize.
		}
		p.flush(ctx)
	}
}

func (p *Publisher) flush(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	// A failed topology capture must not prevent independent lifecycle publication.
	if err := p.publishLifecycle(ctx); err != nil {
		p.warn(err)
	}
	if err := p.publishTopology(ctx); err != nil {
		p.warn(err)
	}
	if p.prunePending && ctx.Err() == nil {
		if err := p.prune(); err != nil {
			p.warn(err)
		}
	}
}

func (p *Publisher) document(kind string, snapshot Snapshot) Document {
	return Document{Format: Format, Version: Version, Kind: kind,
		Producer:    Producer{AgentVersion: buildinfo.Version, RunID: p.runID},
		PublishedAt: time.Now().UTC(), Snapshot: snapshot}
}

func (p *Publisher) publishLifecycle(ctx context.Context) error {
	revision := p.source.LifecycleRevision()
	p.mu.Lock()
	ownerRevision, active := p.revision, p.topology != nil
	p.mu.Unlock()
	if p.lifecycleWritten && p.lifecycleRevision == revision && p.lifecycleOwnerRevision == ownerRevision {
		return nil
	}
	d := p.document(KindLifecycle, Snapshot{Lifecycle: CaptureLifecycle(p.source)})
	d.TopologyActive = active
	if err := p.writeFile(ctx, filepath.Join(p.directory, LifecycleFilename), d, p.rename); err != nil {
		return err
	}
	p.lifecycleWritten, p.lifecycleRevision, p.lifecycleOwnerRevision = true, revision, ownerRevision
	return nil
}

func (p *Publisher) publishTopology(ctx context.Context) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("topology capture panic: %v", v)
		}
	}()
	// Keep lifecycle publication progressing even if new cuts arrive faster
	// than they can be written. Capture only the sequence, not old references.
	p.mu.Lock()
	through := p.sequence
	p.mu.Unlock()
	for {
		item, ok := p.nextCheckpoint(through)
		if !ok {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !p.initialized {
			entries, err := ListCheckpoints(p.directory)
			if err != nil {
				return err
			}
			if len(entries) > 0 {
				p.fileSequence = entries[len(entries)-1].Sequence
			}
			p.initialized = true
		}
		if p.fileSequence == ^uint64(0) {
			return errors.New("diagnostic checkpoint sequence exhausted")
		}
		snapshot, err := item.checkpoint.Capture()
		if err != nil {
			return err
		}
		d := p.document(KindTopology, snapshot)
		d.Checkpoint = p.fileSequence + 1
		path := filepath.Join(p.directory, TopologyDirectory, checkpointFilename(d.Checkpoint))
		if err := p.writeFile(ctx, path, d, p.rename); err != nil {
			return err
		}
		p.fileSequence = d.Checkpoint
		p.mu.Lock()
		p.pending = slices.DeleteFunc(p.pending, func(queued pendingCheckpoint) bool { return queued.sequence <= item.sequence })
		p.mu.Unlock()
		p.prunePending = true
		if err := p.prune(); err != nil {
			return err
		}
	}
}

// Select again after each write so slow IO pins only the in-flight cut, and
// does not project history that newer submissions have already evicted.
func (p *Publisher) nextCheckpoint(through uint64) (pendingCheckpoint, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.pending) == 0 || p.pending[0].sequence > through {
		return pendingCheckpoint{}, false
	}
	return p.pending[0], true
}

func (p *Publisher) prune() error {
	entries, err := ListCheckpoints(p.directory)
	if err != nil {
		return err
	}
	for len(entries) > CheckpointRetention {
		if err := p.remove(filepath.Join(p.directory, TopologyDirectory, entries[0].Filename)); err != nil {
			return err
		}
		entries = entries[1:]
	}
	p.prunePending = false
	return nil
}

func (p *Publisher) warn(err error) {
	p.Limit("snmp:diagnostic-publication", 1, time.Hour).Warningf("failed to publish SNMP diagnostics: %v", err)
}

func writeArchiveFile(ctx context.Context, path string, document Document, replace func(string, string) error) error {
	return writeArchiveFileWithClose(ctx, path, document, (*os.File).Close, replace)
}

func writeArchiveFileWithClose(
	ctx context.Context,
	path string,
	document Document,
	closeFile func(*os.File) error,
	replace func(string, string) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.Remove(temp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer os.Remove(temp)
	if err := file.Chmod(0o600); err != nil {
		_ = closeFile(file)
		return err
	}
	encodeErr := Write(file, document)
	closeErr := closeFile(file)
	if err := errors.Join(encodeErr, closeErr); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return replace(temp, path)
}
