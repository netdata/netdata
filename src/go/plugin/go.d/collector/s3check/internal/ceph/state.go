// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/probe"
)

func (e *Engine) takeover() error {
	if err := e.journal.MutationError(); err != nil {
		return fmt.Errorf("continue Ceph ownership: %w", err)
	}
	if e.locked {
		return nil
	}
	var authoritative state
	locked, found, err := e.journal.TryTakeover(&authoritative)
	if err != nil {
		return fmt.Errorf("take over Ceph ownership: %w", err)
	}
	if !locked {
		return errors.New("Ceph ownership is held by another runtime")
	}
	if found {
		e.state = authoritative
	} else {
		e.state = state{}
	}
	if err := e.validateState(); err != nil {
		e.journal.Unlock()
		return fmt.Errorf("validate authoritative Ceph ownership: %w", err)
	}
	e.locked = true
	return nil
}

func (e *Engine) entryIndex(key string) int {
	for index := range e.state.Entries {
		if e.state.Entries[index].Key == key {
			return index
		}
	}
	return -1
}

func (e *Engine) active() *entry {
	if e.state.ActiveKey == "" {
		return nil
	}
	for index := range e.state.Entries {
		if e.state.Entries[index].Key == e.state.ActiveKey {
			return &e.state.Entries[index]
		}
	}
	return nil
}

func (e *Engine) removeActive() {
	for index := range e.state.Entries {
		if e.state.Entries[index].Key == e.state.ActiveKey {
			e.state.Entries = append(e.state.Entries[:index], e.state.Entries[index+1:]...)
			break
		}
	}
	e.state.ActiveKey = ""
}

func (e *Engine) moveToCleanup(owned *entry) {
	if owned == nil {
		e.state.ActiveKey = ""
		return
	}
	owned.Phase = phaseCleanup
	base := owned.CreatedAt
	if owned.PutAt != nil {
		base = *owned.PutAt
	}
	owned.CleanupAfter = base.Add(e.writeTimeout)
	if owned.PutAt == nil {
		deadline := e.now().UTC().Add(e.sourceRequestTimeout).Add(e.writeTimeout)
		if deadline.After(owned.CleanupAfter) {
			owned.CleanupAfter = deadline
		}
	}
	if owned.DeleteAt != nil {
		if deadline := owned.DeleteAt.Add(e.deleteTimeout); deadline.After(owned.CleanupAfter) {
			owned.CleanupAfter = deadline
		}
	}
	e.state.ActiveKey = ""
}

func (e *Engine) retire(owned *entry) error {
	previous := cloneState(e.state)
	e.moveToCleanup(owned)
	if err := e.persist(); err != nil {
		e.state = previous
		return err
	}
	return nil
}

func (e *Engine) retireWithResult(owned *entry, result *contract.ProbeResult) *contract.ProbeResult {
	previous := cloneState(e.state)
	e.moveToCleanup(owned)
	e.state.LastTerminal = contract.CloneProbe(result)
	if err := e.persist(); err != nil {
		e.state = previous
	}
	return contract.CloneProbe(result)
}

func (e *Engine) setTerminal(result *contract.ProbeResult) *contract.ProbeResult {
	e.state.LastTerminal = contract.CloneProbe(result)
	return contract.CloneProbe(result)
}

func (e *Engine) persist() error {
	var err error
	if len(e.state.Entries) == 0 {
		err = e.journal.Clear()
	} else {
		err = e.journal.Save(e.state)
	}
	if err == nil {
		return nil
	}
	err = fmt.Errorf("persist Ceph ownership: %w", err)
	e.diagnostic = errors.Join(e.diagnostic, err)
	return err
}

func (e *Engine) validateState() error {
	if len(e.state.Entries) > e.queueCapacity {
		return fmt.Errorf("journal has %d entries, capacity is %d", len(e.state.Entries), e.queueCapacity)
	}
	if e.state.CleanupCursor < 0 {
		return errors.New("journal cleanup cursor is negative")
	}
	seen := make(map[string]struct{}, len(e.state.Entries))
	activeFound := e.state.ActiveKey == ""
	for _, owned := range e.state.Entries {
		switch {
		case !strings.HasPrefix(owned.Key, e.namespace):
			return errors.New("journal entry is outside the owner namespace")
		case owned.CreatedAt.IsZero():
			return errors.New("journal entry has no creation time")
		case !probe.ValidDigest(owned.Digest):
			return errors.New("journal entry has invalid payload digest")
		case owned.Phase != phaseWriteVisibility &&
			owned.Phase != phaseDeleteVisibility &&
			owned.Phase != phaseCleanup:
			return fmt.Errorf("journal entry has invalid phase %q", owned.Phase)
		}
		if _, ok := seen[owned.Key]; ok {
			return errors.New("journal contains a duplicate key")
		}
		seen[owned.Key] = struct{}{}
		if owned.Key == e.state.ActiveKey {
			if owned.Phase == phaseCleanup {
				return errors.New("active journal entry is in cleanup")
			}
			activeFound = true
		} else if owned.Phase != phaseCleanup {
			return errors.New("non-active journal entry is outside cleanup")
		}
	}
	if !activeFound {
		return errors.New("active journal key is missing")
	}
	return nil
}

func cloneState(value state) state {
	cloned := value
	cloned.Entries = append([]entry(nil), value.Entries...)
	cloned.LastTerminal = contract.CloneProbe(value.LastTerminal)
	return cloned
}
