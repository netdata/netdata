// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
)

const MaximumMutationPublicationChanges = jobmgr.MaximumFunctionMutationChanges

type PublicationRecord struct {
	Name               string
	Generation         uint64
	Timeout            int
	Help               string
	Tags               string
	Access             string
	Priority           int
	Version            int
	AvailabilityDigest [32]byte
}

type PublicationHandle struct {
	ID         uint64
	Epoch      uint64
	Generation uint64
	Name       string
}

// PublicationPort is the external Function registry boundary. Calls may block
// and therefore Publication transitions must execute as serialized TaskChild
// work, never on KernelLoop.
type PublicationPort interface {
	Publish(PublicationRecord) (PublicationHandle, error)
	Withdraw(PublicationHandle) error
}

type PublicationChange struct {
	Name   string
	Record *PublicationRecord
}

type publishedRoute struct {
	record PublicationRecord
	handle PublicationHandle
}

type PublicationCensus struct {
	Epoch           uint64
	Version         uint64
	Published       int
	RetainedHandles int
	Stopped         bool
	Dirty           bool
}

// Publication owns the acknowledged external Function registration set. Its
// methods are deliberately synchronous so a caller can serialize them through
// one CommandKernel lane without another worker, timer, or reconciliation
// goroutine.
type Publication struct {
	epoch     uint64
	version   uint64
	digest    [32]byte
	port      PublicationPort
	published map[string]publishedRoute
	retained  []PublicationHandle
	stopped   bool
	dirty     error
}

func NewPublication(epoch uint64, port PublicationPort) (*Publication, error) {
	if epoch == 0 || port == nil {
		return nil, errors.New("jobmgr Function publication: invalid construction")
	}
	return &Publication{epoch: epoch, port: port, published: make(map[string]publishedRoute)}, nil
}

// ApplyInitialSnapshot installs one complete catalog-backed snapshot before
// Function ingress is published. Catalog storage, rather than the steady
// mutation quantum, bounds the snapshot.
func (publication *Publication) ApplyInitialSnapshot(
	epoch, version uint64,
	digest [32]byte,
	catalogStorageBytes int64,
	changes []PublicationChange,
) error {
	if publication == nil {
		return errors.New("jobmgr Function publication: invalid initial snapshot")
	}
	if publication.version != 0 ||
		len(publication.published) != 0 ||
		catalogStorageBytes < 0 ||
		catalogStorageBytes > MaximumCatalogStorageBytes ||
		int64(len(changes)) > catalogStorageBytes {
		return publication.poison(
			errors.New(
				"jobmgr Function publication: invalid initial snapshot",
			),
		)
	}
	for _, change := range changes {
		if change.Record == nil {
			return publication.poison(
				errors.New(
					"jobmgr Function publication: initial snapshot contains a withdrawal",
				),
			)
		}
	}
	return publication.applyTransition(epoch, version, digest, changes, 0, func() error {
		return nil
	}, nil, nil)
}

// ApplyTransition closes predecessor admission, withdraws changed predecessors,
// commits the private catalog transition, and only then publishes successors.
// The caller must serialize this method with all other publication and catalog
// mutations.
func (publication *Publication) ApplyTransition(
	epoch, version uint64,
	digest [32]byte,
	changes []PublicationChange,
	quiesce func() error,
	commit func() error,
	abort func() error,
) error {
	return publication.applyTransition(
		epoch,
		version,
		digest,
		changes,
		MaximumMutationPublicationChanges,
		commit,
		quiesce,
		abort,
	)
}

func (publication *Publication) applyTransition(
	epoch, version uint64,
	digest [32]byte,
	changes []PublicationChange,
	maximumChanges int,
	commit func() error,
	quiesce func() error,
	abort func() error,
) error {
	if commit == nil {
		return errors.New("jobmgr Function publication: nil transition commit")
	}
	if maximumChanges != 0 && (quiesce == nil || abort == nil) {
		return errors.New("jobmgr Function publication: incomplete transition boundary")
	}
	if err := publication.validateTransition(
		epoch,
		version,
		changes,
		maximumChanges,
	); err != nil {
		return err
	}
	if quiesce != nil {
		if err := quiesce(); err != nil {
			return publication.poison(err)
		}
	}
	for _, change := range changes {
		current, exists := publication.published[change.Name]
		if !exists || (change.Record != nil && current.record == *change.Record) {
			continue
		}
		if err := publication.port.Withdraw(current.handle); err != nil {
			return publication.poison(errors.Join(err, abort()))
		}
		delete(publication.published, change.Name)
	}
	if err := commit(); err != nil {
		return publication.poison(err)
	}
	for _, change := range changes {
		if change.Record == nil {
			continue
		}
		current, exists := publication.published[change.Name]
		if exists && current.record == *change.Record {
			continue
		}
		handle, err := publication.port.Publish(*change.Record)
		if err != nil {
			return publication.poison(err)
		}
		if err := publication.validateHandle(handle, *change.Record); err != nil {
			if handle.ID != 0 {
				publication.retained = append(publication.retained, handle)
			}
			return publication.poison(err)
		}
		next := publishedRoute{record: *change.Record, handle: handle}
		publication.published[change.Name] = next
	}
	publication.version = version
	publication.digest = digest
	return nil
}

func (publication *Publication) validateTransition(
	epoch, version uint64,
	changes []PublicationChange,
	maximumChanges int,
) error {
	if publication == nil || publication.port == nil {
		return errors.New("jobmgr Function publication: invalid state")
	}
	if publication.dirty != nil {
		return publication.dirty
	}
	if publication.stopped {
		return errors.New("jobmgr Function publication: apply after stop")
	}
	if epoch != publication.epoch ||
		version == 0 ||
		version != publication.version+1 ||
		maximumChanges < 0 ||
		maximumChanges != 0 && len(changes) > maximumChanges {
		return publication.poison(errors.New("jobmgr Function publication: stale or invalid transition"))
	}
	seen := make(map[string]struct{}, len(changes))
	for _, change := range changes {
		if change.Name == "" {
			return publication.poison(errors.New("jobmgr Function publication: empty route name"))
		}
		if _, ok := seen[change.Name]; ok {
			return publication.poison(errors.New("jobmgr Function publication: duplicate route change"))
		}
		seen[change.Name] = struct{}{}
		_, exists := publication.published[change.Name]
		if change.Record == nil {
			if !exists {
				return publication.poison(errors.New("jobmgr Function publication: withdraw of unpublished route"))
			}
			continue
		}
		if change.Record.Name != change.Name || change.Record.Generation == 0 ||
			change.Record.Timeout < 0 || change.Record.Priority < 0 ||
			change.Record.Version < 0 || change.Record.Access == "" {
			return publication.poison(errors.New("jobmgr Function publication: invalid desired record"))
		}
	}
	return nil
}

func (publication *Publication) validateHandle(handle PublicationHandle, record PublicationRecord) error {
	if handle.ID == 0 || handle.Epoch != publication.epoch ||
		handle.Generation != record.Generation || handle.Name != record.Name {
		return errors.New("jobmgr Function publication: mismatched acknowledgement handle")
	}
	return nil
}

func (publication *Publication) Poll(epoch, version uint64, digest [32]byte) error {
	if publication == nil {
		return errors.New("jobmgr Function publication: nil poll")
	}
	if publication.dirty != nil {
		return publication.dirty
	}
	if publication.stopped {
		return errors.New("jobmgr Function publication: poll after stop")
	}
	if epoch != publication.epoch || version != publication.version || digest != publication.digest {
		return publication.poison(errors.New("jobmgr Function publication: stale availability poll"))
	}
	return nil
}

func (publication *Publication) Stop(epoch uint64) error {
	if publication == nil {
		return nil
	}
	if publication.stopped && len(publication.published) == 0 && len(publication.retained) == 0 {
		return publication.dirty
	}
	publication.stopped = true
	var stopErr error
	if epoch != publication.epoch {
		stopErr = publication.poison(errors.New("jobmgr Function publication: stop epoch mismatch"))
	}
	for name, current := range publication.published {
		if err := publication.port.Withdraw(current.handle); err != nil {
			stopErr = errors.Join(stopErr, err)
			continue
		}
		delete(publication.published, name)
	}
	retained := publication.retained[:0]
	for _, handle := range publication.retained {
		if err := publication.port.Withdraw(handle); err != nil {
			stopErr = errors.Join(stopErr, err)
			retained = append(retained, handle)
		}
	}
	publication.retained = retained
	return errors.Join(publication.dirty, stopErr)
}

func (publication *Publication) Census() PublicationCensus {
	if publication == nil {
		return PublicationCensus{}
	}
	return PublicationCensus{
		Epoch: publication.epoch, Version: publication.version,
		Published: len(publication.published), RetainedHandles: len(publication.retained),
		Stopped: publication.stopped, Dirty: publication.dirty != nil,
	}
}

func (publication *Publication) poison(err error) error {
	if err == nil {
		return publication.dirty
	}
	publication.dirty = errors.Join(publication.dirty, err)
	return publication.dirty
}

func DigestSortedPublications(records []PublicationRecord) ([32]byte, error) {
	digest := sha256.New()
	for index, record := range records {
		if record.Name == "" || record.Generation == 0 || record.Timeout < 0 ||
			record.Priority < 0 || record.Version < 0 ||
			(index > 0 && records[index-1].Name >= record.Name) {
			return [32]byte{}, errors.New("jobmgr Function publication: invalid or unsorted records")
		}
		writeDigestString(digest, record.Name)
		writeDigestUint64(digest, record.Generation)
		writeDigestUint64(digest, uint64(record.Timeout))
		writeDigestString(digest, record.Help)
		writeDigestString(digest, record.Tags)
		writeDigestString(digest, record.Access)
		writeDigestUint64(digest, uint64(record.Priority))
		writeDigestUint64(digest, uint64(record.Version))
		_, _ = digest.Write(record.AvailabilityDigest[:])
	}
	var result [32]byte
	copy(result[:], digest.Sum(nil))
	return result, nil
}

func writeDigestString(digest hash.Hash, value string) {
	writeDigestUint64(digest, uint64(len(value)))
	_, _ = digest.Write([]byte(value))
}

func writeDigestUint64(digest hash.Hash, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = digest.Write(encoded[:])
}
