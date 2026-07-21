// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import "errors"

type PublicationRecord struct {
	Name       string // public Function name
	Generation uint64 // handler generation exposed
	Timeout    int    // advertised timeout
	Help       string // help text
	Tags       string // advertised tags
	Access     string // access mask
	Priority   int    // advertised priority
	Version    int    // advertised version
}

// PublicationPort is the external Function registry boundary. Calls may block
// and therefore Publication transitions must execute as serialized TaskChild
// work, never on the kernel loop.
type PublicationPort interface {
	Publish(PublicationRecord) error
	Withdraw(string) error
}

type PublicationChange struct {
	Name   string
	Record *PublicationRecord
}

// Publication owns the acknowledged external Function registration set. Its
// methods are deliberately synchronous so a caller can serialize them through
// one CommandKernel lane without another worker, timer, or reconciliation
// goroutine.
type Publication struct {
	epoch     uint64                       // run epoch this publication belongs to
	version   uint64                       // publication version
	port      PublicationPort              // wire port emitting FUNCTION/FUNCTION_DEL
	published map[string]PublicationRecord // currently published routes by name
	stopped   bool                         // publication stopped (shutdown)
	dirty     error                        // sticky poison error
}

func NewPublication(epoch uint64, port PublicationPort) (*Publication, error) {
	if epoch == 0 || port == nil {
		return nil, errors.New("jobmgr Function publication: invalid construction")
	}
	return &Publication{epoch: epoch, port: port, published: make(map[string]PublicationRecord)}, nil
}

// ApplyInitialSnapshot installs one complete catalog-backed snapshot before
// Function ingress is published.
func (p *Publication) ApplyInitialSnapshot(
	epoch, version uint64,
	changes []PublicationChange,
) error {
	if p == nil {
		return errors.New("jobmgr Function publication: invalid initial snapshot")
	}
	if p.version != 0 ||
		len(p.published) != 0 {
		return p.poison(
			errors.New(
				"jobmgr Function publication: invalid initial snapshot",
			),
		)
	}
	for _, change := range changes {
		if change.Record == nil {
			return p.poison(
				errors.New(
					"jobmgr Function publication: initial snapshot contains a withdrawal",
				),
			)
		}
	}
	return p.applyTransition(epoch, version, changes, func() error {
		return nil
	}, nil, nil)
}

// ApplyTransition closes predecessor admission, withdraws changed predecessors,
// commits the private catalog transition, and only then publishes successors.
// The caller must serialize this method with all other publication and catalog
// mutations.
func (p *Publication) ApplyTransition(
	epoch, version uint64,
	changes []PublicationChange,
	quiesce func() error,
	commit func() error,
	abort func() error,
) error {
	if quiesce == nil || abort == nil {
		return errors.New("jobmgr Function publication: incomplete transition boundary")
	}
	return p.applyTransition(
		epoch,
		version,
		changes,
		commit,
		quiesce,
		abort,
	)
}

func (p *Publication) applyTransition(
	epoch, version uint64,
	changes []PublicationChange,
	commit func() error,
	quiesce func() error,
	abort func() error,
) error {
	if commit == nil {
		return errors.New("jobmgr Function publication: nil transition commit")
	}
	if (quiesce == nil) != (abort == nil) {
		return errors.New("jobmgr Function publication: incomplete transition boundary")
	}
	if err := p.validateTransition(
		epoch,
		version,
		changes,
	); err != nil {
		return err
	}
	if quiesce != nil {
		if err := quiesce(); err != nil {
			return p.poison(err)
		}
	}
	for _, change := range changes {
		current, exists := p.published[change.Name]
		if !exists || (change.Record != nil && current == *change.Record) {
			continue
		}
		if err := p.port.Withdraw(change.Name); err != nil {
			var abortErr error
			if abort != nil {
				abortErr = abort()
			}
			return p.poison(errors.Join(err, abortErr))
		}
		delete(p.published, change.Name)
	}
	if err := commit(); err != nil {
		return p.poison(err)
	}
	for _, change := range changes {
		if change.Record == nil {
			continue
		}
		current, exists := p.published[change.Name]
		if exists && current == *change.Record {
			continue
		}
		if err := p.port.Publish(*change.Record); err != nil {
			return p.poison(err)
		}
		p.published[change.Name] = *change.Record
	}
	p.version = version
	return nil
}

func (p *Publication) validateTransition(
	epoch, version uint64,
	changes []PublicationChange,
) error {
	if p == nil || p.port == nil {
		return errors.New("jobmgr Function publication: invalid state")
	}
	if p.dirty != nil {
		return p.dirty
	}
	if p.stopped {
		return errors.New("jobmgr Function publication: apply after stop")
	}
	if epoch != p.epoch ||
		version == 0 ||
		version != p.version+1 {
		return p.poison(errors.New("jobmgr Function publication: stale or invalid transition"))
	}
	seen := make(map[string]struct{}, len(changes))
	for _, change := range changes {
		if change.Name == "" {
			return p.poison(errors.New("jobmgr Function publication: empty route name"))
		}
		if _, ok := seen[change.Name]; ok {
			return p.poison(errors.New("jobmgr Function publication: duplicate route change"))
		}
		seen[change.Name] = struct{}{}
		_, exists := p.published[change.Name]
		if change.Record == nil {
			if !exists {
				return p.poison(errors.New("jobmgr Function publication: withdraw of unpublished route"))
			}
			continue
		}
		if change.Record.Name != change.Name || change.Record.Generation == 0 ||
			change.Record.Timeout < 0 || change.Record.Priority < 0 ||
			change.Record.Version < 0 || change.Record.Access == "" {
			return p.poison(errors.New("jobmgr Function publication: invalid desired record"))
		}
	}
	return nil
}

func (p *Publication) Stop(epoch uint64) error {
	if p == nil {
		return nil
	}
	if p.stopped && len(p.published) == 0 {
		return p.dirty
	}
	p.stopped = true
	var stopErr error
	if epoch != p.epoch {
		stopErr = p.poison(errors.New("jobmgr Function publication: stop epoch mismatch"))
	}
	for name := range p.published {
		if err := p.port.Withdraw(name); err != nil {
			stopErr = errors.Join(stopErr, err)
			continue
		}
		delete(p.published, name)
	}
	return errors.Join(p.dirty, stopErr)
}

func (p *Publication) poison(err error) error {
	if err == nil {
		return p.dirty
	}
	p.dirty = errors.Join(p.dirty, err)
	return p.dirty
}
