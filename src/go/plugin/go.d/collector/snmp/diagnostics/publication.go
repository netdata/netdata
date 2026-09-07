// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
)

const DefaultInterval = 30 * time.Minute
const firstEvidenceCheckEvery = time.Minute

func ArchivePath(varLibDir string) string {
	return filepath.Join(varLibDir, "snmp-topology", "diagnostics", "netdata-snmp-topology-diagnostics.zst")
}
func defaultArchivePath(varLibDir string) string {
	dir := strings.TrimSpace(varLibDir)
	if dir == "" {
		dir = strings.TrimSpace(buildinfo.VarLibDir)
	}
	if dir == "" {
		dir = buildinfo.DefaultVarLibDir
	}
	return ArchivePath(filepath.Clean(dir))
}

type Source interface {
	LifecycleSource
	ConfigurationRevision() uint64
	ConfigurationChanges() <-chan struct{}
}

// TopologySource exposes only diagnostic snapshots, without connection state.
type TopologySource interface{ Capture() (Snapshot, error) }

type Publisher struct {
	*logger.Logger
	source                    Source
	path                      string
	mu                        sync.Mutex
	topology                  TopologySource
	owner                     string
	revision                  uint64
	publishedTopologyRevision uint64
	interval                  time.Duration
	changed                   chan struct{}
	rename                    func(string, string) error
	writeFile                 func(context.Context, string, Document, func(string, string) error) error
}

func NewPublisher(source Source, varLibDir string) *Publisher {
	return &Publisher{
		Logger:    logger.New(),
		source:    source,
		path:      defaultArchivePath(varLibDir),
		interval:  DefaultInterval,
		changed:   make(chan struct{}, 1),
		writeFile: writeArchiveFile,
		rename:    os.Rename,
	}
}

// SetTopology is called only for an accepted configuration. Candidate captures
// stay private until this boundary.
func (p *Publisher) SetTopology(owner string, source TopologySource, interval time.Duration) {
	p.mu.Lock()
	p.owner = owner
	p.topology = source
	p.revision++
	p.interval = DefaultInterval
	if source != nil && interval > 0 {
		p.interval = interval
	}
	p.mu.Unlock()
	p.notify()
}
func (p *Publisher) RemoveTopology(owner string) {
	p.mu.Lock()
	if p.owner == owner {
		p.owner = ""
		p.topology = nil
		p.interval = DefaultInterval
		p.revision++
	}
	p.mu.Unlock()
	p.notify()
}
func (p *Publisher) ReleaseTopology(source TopologySource) {
	p.mu.Lock()
	if p.topology == source {
		p.owner = ""
		p.topology = nil
		p.interval = DefaultInterval
		p.revision++
	}
	p.mu.Unlock()
	p.notify()
}

// TopologyUpdated must remain independent of file replacement: collectors call
// it while committing a generation. The writer filters coalesced notifications.
func (p *Publisher) TopologyUpdated() { p.notify() }
func (p *Publisher) needsInitialTopology() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.topology != nil && p.publishedTopologyRevision != p.revision
}
func (p *Publisher) notify() {
	select {
	case p.changed <- struct{}{}:
	default:
	}
}
func (p *Publisher) currentInterval() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.interval
}

func (p *Publisher) Run(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	meaningful := p.publish(ctx, false)
	p.mu.Lock()
	scheduleRevision := p.revision
	p.mu.Unlock()
	periodic := time.NewTimer(p.currentInterval())
	defer periodic.Stop()
	first := time.NewTicker(firstEvidenceCheckEvery)
	defer first.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.changed:
			p.mu.Lock()
			revision := p.revision
			p.mu.Unlock()
			if scheduleRevision != revision {
				periodic.Reset(p.currentInterval())
				scheduleRevision = revision
			}
			if !meaningful || p.needsInitialTopology() {
				meaningful = p.publish(ctx, true) || meaningful
			}
		case <-p.source.ConfigurationChanges():
			if !meaningful || p.needsInitialTopology() {
				meaningful = p.publish(ctx, true) || meaningful
			}
		case <-first.C:
			if !meaningful || p.needsInitialTopology() {
				meaningful = p.publish(ctx, true) || meaningful
			}
		case <-periodic.C:
			meaningful = p.publish(ctx, false) || meaningful
			periodic.Reset(p.currentInterval())
		}
	}
}

func (p *Publisher) publish(ctx context.Context, requireMeaningful bool) (meaningful bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			meaningful = false
			p.warn(fmt.Errorf("capture panic: %v", recovered))
		}
	}()
	if ctx.Err() != nil {
		return false
	}
	configRevision := p.source.ConfigurationRevision()
	p.mu.Lock()
	source, revision := p.topology, p.revision
	p.mu.Unlock()
	snapshot := Snapshot{}
	if source != nil {
		var err error
		snapshot, err = source.Capture()
		if err != nil {
			p.warn(err)
			return false
		}
	} else {
		snapshot.Lifecycle = CaptureLifecycle(p.source, MaxRecords, MaxLogicalBytes)
	}
	meaningful = len(snapshot.Lifecycle.Cut.Entries) > 0
	if requireMeaningful && !meaningful {
		return false
	}
	document := Document{
		Format:  Format,
		Version: Version,
		Producer: Producer{
			AgentVersion: buildinfo.Version,
		},
		Snapshot: snapshot,
	}
	err := p.writeFile(ctx, p.path, document, func(from, to string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.mu.Lock()
		current := p.revision == revision
		p.mu.Unlock()
		if !current || p.source.ConfigurationRevision() != configRevision {
			return errors.New("diagnostic ownership changed during publication")
		}
		// This is a historical checkpoint, not a live inventory pointer. A
		// concurrent configuration change may leave this cut on disk; no
		// collection or activation lock may be held during filesystem I/O.
		if err := p.rename(from, to); err != nil {
			return err
		}
		p.mu.Lock()
		if p.revision == revision && snapshot.Topology != nil {
			p.publishedTopologyRevision = revision
		}
		p.mu.Unlock()
		return nil
	})
	if err != nil {
		p.warn(err)
		return false
	}
	return meaningful
}
func (p *Publisher) warn(err error) {
	p.Limit("snmp:diagnostic-archive", 1, time.Hour).Warningf("failed to publish SNMP diagnostic archive: %v", err)
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
