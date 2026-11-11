// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

var ErrNativeClosed = errors.New("native browser runtime admission is closed; repair or configure its package")

// NativeConfig fixes one package prefix and its permanent control/proof root.
// All supported consumers of this package must use this same configuration.
type NativeConfig struct {
	ControlPath string
	PayloadPath string
}

// DefaultNativeConfig is package wiring, not a per-consumer configuration knob.
func DefaultNativeConfig() NativeConfig {
	return NativeConfig{ControlPath: "/var/lib/netdata-dem-runtime", PayloadPath: "/usr/lib/netdata/dem-runtime"}
}

// NativeStatus reports service-owned lifetime proof without requiring a healthy
// package payload. Root package hooks obtain it by invoking the service UID.
type NativeStatus struct {
	ProcessClaims   int    `json:"process_claims"`
	CandidateClaims int    `json:"candidate_claims"`
	ExecutionPins   int    `json:"execution_pins"`
	UncertainPins   int    `json:"uncertain_pins"`
	InventoryError  string `json:"inventory_error,omitempty"`
	CleanupError    string `json:"cleanup_error,omitempty"`
}

func (s NativeStatus) empty() error {
	if s.InventoryError != "" || s.CleanupError != "" {
		return errors.New("native ownership inspection or cleanup is incomplete")
	}
	if s.ProcessClaims != 0 || s.CandidateClaims != 0 || s.ExecutionPins != 0 || s.UncertainPins != 0 {
		return ErrInUse
	}
	return nil
}

// Native owns only service-private lifetime proof. It never modifies package
// files or root admission state, and never initializes an established ledger.
type Native struct {
	config NativeConfig
	ledger *ownershipLedger
}

func OpenNative(config NativeConfig) (*Native, error) {
	state, err := readNativeControl(config)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_, controlErr := os.Lstat(config.ControlPath)
			_, payloadErr := os.Lstat(config.PayloadPath)
			if errors.Is(controlErr, os.ErrNotExist) && errors.Is(payloadErr, os.ErrNotExist) {
				return nil, ErrUnavailable
			}
		}
		return nil, err
	}
	if err := nativeServiceIdentity(state); err != nil {
		return nil, err
	}
	if state.Phase == "initializing" {
		return nil, ErrNativeClosed
	}
	path := filepath.Join(config.ControlPath, "owners")
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	record, err := ownershipRootLayout.read(root)
	if err == nil && record.Phase != "ready" {
		err = errors.New("native ownership initialization is incomplete")
	}
	if err == nil {
		err = ownershipRootLayout.validate(root)
	}
	if err != nil {
		return nil, errors.Join(err, root.Close())
	}
	// Keep the validated root. An established native owner must never enter
	// bootstrap through a second open, even if state disappears in between.
	return &Native{config: config, ledger: newOwnershipLedger(path, root)}, nil
}

// InitializeNativeProof is invoked as the service UID only by root bootstrap.
// The root caller holds the gate; this command must not acquire it again.
func InitializeNativeProof(config NativeConfig) error {
	state, err := readNativeControl(config)
	if err != nil {
		return err
	}
	if err := nativeServiceIdentity(state); err != nil {
		return err
	}
	if state.Phase != "initializing" {
		return errors.New("native proof bootstrap is not authorized by the root gate")
	}
	ledger, err := openOwnershipLedger(filepath.Join(config.ControlPath, "owners"))
	if err != nil {
		return err
	}
	return ledger.Close()
}

func (n *Native) Close() error { return n.ledger.Close() }

func (n *Native) Inspect() (NativeStatus, error) {
	var status NativeStatus
	err := n.ledger.withLock(func() error {
		inv, err := n.ledger.inventoryLocked(false)
		if err != nil {
			return err
		}
		for _, count := range inv.processes {
			status.ProcessClaims += count
		}
		for _, count := range inv.candidates {
			status.CandidateClaims += count
		}
		for _, count := range inv.uses {
			status.ExecutionPins += count
		}
		for _, count := range inv.uncertain {
			status.UncertainPins += count
		}
		if err := errors.Join(inv.cleanupError, n.ledger.cleanupError); err != nil {
			status.CleanupError = err.Error()
		}
		return nil
	})
	if err != nil {
		status.InventoryError = err.Error()
	}
	return status, err
}

// Sweep reconciles proof while root admission may be closed. It cannot reopen
// admission or remove executable files, including after verified reboot.
func (n *Native) Sweep() error {
	var names []string
	var version uint64
	err := n.ledger.withLock(func() error {
		version = n.ledger.cleanupErrorVersion
		if err := n.ledger.retryOwnerReleasesLocked(); err != nil {
			return err
		}
		inv, err := n.ledger.inventoryLocked(true)
		if err != nil {
			return err
		}
		names, err = n.ledger.trashSnapshotLocked()
		return errors.Join(err, inv.cleanupError)
	})
	if err == nil {
		err = n.ledger.deleteTrash(names)
	}
	n.ledger.finishSweep(version, err)
	return err
}

// NativeConsumer holds one process or qualification claim for a fixed package
// target. Qualification retains this handle across all checks and their gaps.
type NativeConsumer struct {
	native   *Native
	manifest Manifest
	paths    Paths
	claim    *claim
	phase    string
	mu       sync.Mutex
	closed   bool
}

func (n *Native) AcquireSelected() (*NativeConsumer, error) {
	return n.acquire("ready", nil)
}

func (n *Native) AcquireCandidate(target Manifest) (*NativeConsumer, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	return n.acquire("qualifying", &target)
}

func (n *Native) acquire(phase string, expected *Manifest) (consumer *NativeConsumer, result error) {
	gate, err := lockNativeGate(context.Background(), n.config, false)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := gate.Close(); err != nil {
			result = errors.Join(result, err)
			if consumer != nil {
				result = errors.Join(result, consumer.Close())
				consumer = nil
			}
		}
	}()
	state, err := readNativeControl(n.config)
	if err != nil {
		return nil, err
	}
	if state.Phase != phase || state.Target == nil {
		return nil, ErrNativeClosed
	}
	if expected != nil && !sameNativeManifest(*state.Target, *expected) {
		return nil, errors.New("native qualification target differs from the root gate")
	}
	paths, m, err := readNativePayload(n.config.PayloadPath)
	if err != nil {
		return nil, err
	}
	if !sameNativeManifest(m, *state.Target) {
		return nil, errors.New("native payload differs from the admitted target")
	}
	var owner *claim
	err = n.ledger.withLock(func() error {
		var err error
		kind := "process"
		if phase == "qualifying" {
			kind = "candidate"
		}
		owner, err = n.ledger.claimLocked(m.Archive.SHA256, kind)
		return err
	})
	if err != nil {
		if owner != nil {
			err = errors.Join(err, owner.Close())
		}
		return nil, err
	}
	return &NativeConsumer{native: n, manifest: m, paths: paths, claim: owner, phase: phase}, nil
}

func (c *NativeConsumer) Manifest() Manifest { return c.manifest }

func (c *NativeConsumer) BeginUse() (use *Use, result error) {
	// Gate precedes consumer/ledger locks. Close and Finish never acquire it.
	gate, err := lockNativeGate(context.Background(), c.native.config, false)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := gate.Close(); err != nil {
			result = errors.Join(result, err)
			if use != nil {
				result = errors.Join(result, use.Finish(true))
				use = nil
			}
		}
	}()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, errors.New("native runtime consumer is closed")
	}
	state, err := readNativeControl(c.native.config)
	if err != nil {
		return nil, err
	}
	if state.Phase != c.phase || state.Target == nil || !sameNativeManifest(*state.Target, c.manifest) {
		return nil, ErrNativeClosed
	}
	err = c.native.ledger.withLock(func() error {
		lease, err := c.native.ledger.newUseLocked(c.manifest.Archive.SHA256)
		if err != nil {
			return err
		}
		if err := lease.publishLocked(); err != nil {
			return errors.Join(err, lease.abortLocked())
		}
		use = &Use{executionLease: lease, Paths: c.paths}
		return nil
	})
	if err != nil {
		if use != nil {
			err = errors.Join(err, use.Finish(true))
			use = nil
		}
		return nil, err
	}
	return use, nil
}

func (c *NativeConsumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return c.claim.Close()
}
