// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
)

func (e *Engine) takeover() error {
	if err := e.journal.MutationError(); err != nil {
		return fmt.Errorf("continue lifecycle ownership: %w", err)
	}
	if e.locked {
		return nil
	}
	var authoritative state
	locked, found, err := e.journal.TryTakeover(&authoritative)
	if err != nil {
		return fmt.Errorf("take over lifecycle ownership: %w", err)
	}
	if !locked {
		return errors.New("lifecycle ownership is held by another runtime")
	}
	if found {
		e.state = authoritative
	} else {
		e.state = state{}
	}
	if err := e.validateState(); err != nil {
		e.journal.Unlock()
		return fmt.Errorf("validate authoritative lifecycle ownership: %w", err)
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

func (e *Engine) finish(result *contract.ProbeResult) *contract.ProbeResult {
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
	err = fmt.Errorf("persist lifecycle ownership: %w", err)
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
	for _, owned := range e.state.Entries {
		if !strings.HasPrefix(owned.Key, e.namespace) {
			return errors.New("journal entry is outside the owner namespace")
		}
		if _, ok := seen[owned.Key]; ok {
			return errors.New("journal contains a duplicate key")
		}
		seen[owned.Key] = struct{}{}
	}
	return nil
}
