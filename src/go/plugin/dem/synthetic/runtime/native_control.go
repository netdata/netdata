// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

const nativeGateName = "coordination.lock"
const nativeStateName = "admission.json"

// NativePackage is installed with the package payload, after build-time binding
// to Path. Native package/repository authentication is its trust boundary.
type NativePackage struct {
	Format   int      `json:"format"`
	Path     string   `json:"path"`
	Manifest Manifest `json:"manifest"`
}

type nativeControlRecord struct {
	Format      int       `json:"format"`
	Phase       string    `json:"phase"`
	PayloadPath string    `json:"payload_path"`
	UID         int       `json:"uid"`
	GID         int       `json:"gid"`
	Target      *Manifest `json:"target,omitempty"`
}

// NativeInspector invokes the installed proof inspector as the configured
// service UID. It must not read service-owned receipt files as root or reacquire
// the root gate: the caller holds that gate continuously through inspection.
type NativeInspector func(context.Context) (NativeStatus, error)

// NativeQualifier invokes service-UID qualification for this exact target. It
// returns success only after all checks pass and its candidate/uses are closed.
type NativeQualifier func(context.Context, Manifest) error

// NativeControl is hook-internal root administration. The package manager must
// serialize the complete lifecycle, including the interval between closing
// admission and unpacking. This API is not a concurrent operator installer.
type NativeControl struct{ config NativeConfig }

func OpenNativeControl(config NativeConfig) (*NativeControl, error) {
	if err := nativeRootIdentity(); err != nil {
		return nil, err
	}
	if _, err := readNativeControl(config); err != nil {
		return nil, err
	}
	return &NativeControl{config: config}, nil
}

// InitializeNativeControl creates a previously absent permanent control root.
// initialize invokes InitializeNativeProof as uid/gid while root holds the gate.
// Interrupted bootstrap resumes only from its initializing record; established
// state never recreates a missing gate, ledger or ownership namespace.
func InitializeNativeControl(ctx context.Context, config NativeConfig, uid, gid int, initialize func(context.Context) error) (_ *NativeControl, result error) {
	if err := nativeRootIdentity(); err != nil {
		return nil, err
	}
	if err := validateNativeConfig(config); err != nil {
		return nil, err
	}
	if uid <= 0 || gid < 0 || initialize == nil {
		return nil, errors.New("native bootstrap requires service UID/GID and initializer")
	}
	if _, err := os.Lstat(config.ControlPath); errors.Is(err, os.ErrNotExist) {
		// First installation closes admission before payload unpack. Existing
		// package files plus missing control state are lost proof, not bootstrap.
		if _, err := os.Lstat(config.PayloadPath); !errors.Is(err, os.ErrNotExist) {
			return nil, errors.Join(errors.New("cannot bootstrap missing native control beside an existing payload"), err)
		}
		if err := createNativeControl(config, uid, gid); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	// A preceding initializer may have died after publishing the directory but
	// before its parent sync. Complete that barrier before proof can be used.
	parent, err := os.OpenRoot(filepath.Dir(config.ControlPath))
	if err != nil {
		return nil, err
	}
	if err := errors.Join(syncDirectory(parent, "."), parent.Close()); err != nil {
		return nil, err
	}
	gate, err := lockNativeGate(ctx, config, true)
	if err != nil {
		return nil, err
	}
	defer func() { result = errors.Join(result, gate.Close()) }()
	state, err := readNativeControl(config)
	if err != nil {
		return nil, err
	}
	if state.UID != uid || state.GID != gid {
		return nil, errors.New("native service identity differs from established control state")
	}
	if state.Phase == "initializing" {
		if err := initialize(ctx); err != nil {
			return nil, err
		}
		state.Phase = "closed"
		if err := writeNativeControl(config, state); err != nil {
			return nil, err
		}
	}
	return &NativeControl{config: config}, nil
}

// CloseAdmission refuses a busy or uncertain installation without changing its
// state. Success durably closes admission before the package may mutate files.
func (c *NativeControl) CloseAdmission(ctx context.Context, inspect NativeInspector) (result error) {
	if err := nativeRootIdentity(); err != nil {
		return err
	}
	gate, err := lockNativeGate(ctx, c.config, true)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, gate.Close()) }()
	state, err := readNativeControl(c.config)
	if err != nil {
		return err
	}
	if state.Phase == "initializing" {
		return errors.New("native bootstrap is incomplete")
	}
	if err := inspectNativeEmpty(ctx, inspect); err != nil {
		return err
	}
	state.Phase, state.Target = "closed", nil
	return writeNativeControl(c.config, state)
}

// QualifyAndReady validates complete package files, admits only a candidate for
// their exact target, and opens ordinary admission after successful qualification
// and a second empty-proof check. Failed/interrupted qualification remains closed
// to ordinary consumers; retry reruns qualification, never infers success.
func (c *NativeControl) QualifyAndReady(ctx context.Context, inspect NativeInspector, qualify NativeQualifier) error {
	if err := nativeRootIdentity(); err != nil {
		return err
	}
	if qualify == nil {
		return errors.New("native qualification callback is required")
	}
	gate, err := lockNativeGate(ctx, c.config, true)
	if err != nil {
		return err
	}
	state, err := readNativeControl(c.config)
	if err != nil {
		return errors.Join(err, gate.Close())
	}
	if state.Phase == "initializing" {
		return errors.Join(errors.New("native bootstrap is incomplete"), gate.Close())
	}
	if err := inspectNativeEmpty(ctx, inspect); err != nil {
		return errors.Join(err, gate.Close())
	}
	_, target, err := readNativePayload(c.config.PayloadPath)
	if err != nil {
		return errors.Join(err, gate.Close())
	}
	if state.Phase == "ready" {
		if state.Target == nil || !sameNativeManifest(*state.Target, target) {
			err = errors.New("native package changed without closed admission")
		}
		return errors.Join(err, gate.Close())
	}
	state.Phase, state.Target = "qualifying", &target
	err = errors.Join(writeNativeControl(c.config, state), gate.Close())
	if err != nil {
		return err
	}
	if err := qualify(ctx, target); err != nil {
		return err
	}
	gate, err = lockNativeGate(ctx, c.config, true)
	if err != nil {
		return err
	}
	state, err = readNativeControl(c.config)
	if err == nil && (state.Phase != "qualifying" || state.Target == nil || !sameNativeManifest(*state.Target, target)) {
		err = errors.New("native qualification admission changed before completion")
	}
	if err == nil {
		_, actual, readErr := readNativePayload(c.config.PayloadPath)
		err = readErr
		if err == nil && !sameNativeManifest(actual, target) {
			err = errors.New("native payload changed during qualification")
		}
	}
	if err == nil {
		err = inspectNativeEmpty(ctx, inspect)
	}
	if err == nil {
		state.Phase = "ready"
		err = writeNativeControl(c.config, state)
	}
	return errors.Join(err, gate.Close())
}

func inspectNativeEmpty(ctx context.Context, inspect NativeInspector) error {
	if inspect == nil {
		return errors.New("native service-UID inspector is required")
	}
	if err := checkContext(ctx); err != nil {
		return err
	}
	status, err := inspect(ctx)
	if err != nil {
		return err
	}
	return status.empty()
}

func lockNativeGate(ctx context.Context, config NativeConfig, exclusive bool) (*flock.Flock, error) {
	if _, err := readNativeControl(config); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lock := flock.New(filepath.Join(config.ControlPath, nativeGateName), flock.SetFlag(os.O_RDONLY))
	var held bool
	var err error
	if exclusive {
		held, err = lock.TryLockContext(ctx, 25*time.Millisecond)
	} else {
		held, err = lock.TryRLockContext(ctx, 25*time.Millisecond)
	}
	if !held || err != nil {
		return nil, errors.Join(err, ctx.Err(), lock.Close())
	}
	// Another writer may have died after rename before syncing admission state.
	root, err := os.OpenRoot(config.ControlPath)
	if err == nil {
		err = errors.Join(syncDirectory(root, "."), root.Close())
	}
	if err != nil {
		return nil, errors.Join(err, lock.Close())
	}
	return lock, nil
}

func readNativeControl(config NativeConfig) (nativeControlRecord, error) {
	var state nativeControlRecord
	if err := validateNativeConfig(config); err != nil {
		return state, err
	}
	if err := nativeProtectedPath(config.ControlPath); err != nil {
		return state, err
	}
	root, err := os.OpenRoot(config.ControlPath)
	if err != nil {
		return state, err
	}
	defer root.Close()
	if err := nativeProtectedFile(root, nativeGateName, true); err != nil {
		return state, err
	}
	if err := nativeProtectedFile(root, nativeStateName, false); err != nil {
		return state, err
	}
	if err := readJSON(root, nativeStateName, &state); err != nil {
		return state, err
	}
	if state.Format != 1 || state.PayloadPath != config.PayloadPath || state.UID <= 0 || state.GID < 0 {
		return state, errors.New("invalid native control identity")
	}
	switch state.Phase {
	case "initializing", "closed":
		if state.Target != nil {
			return state, errors.New("closed native gate has an executable target")
		}
	case "qualifying", "ready":
		if state.Target == nil {
			return state, errors.New("native gate is missing its target")
		}
		if err := state.Target.Validate(); err != nil {
			return state, err
		}
	default:
		return state, errors.New("unknown native admission phase")
	}
	if err := nativeProofDirectory(config, state); err != nil {
		return state, err
	}
	return state, nil
}

func writeNativeControl(config NativeConfig, state nativeControlRecord) error {
	root, err := os.OpenRoot(config.ControlPath)
	if err != nil {
		return err
	}
	defer root.Close()
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	name := ".admission-" + id
	if err := writeNewFile(root, name, append(data, '\n'), 0644); err != nil {
		return err
	}
	defer root.Remove(name)
	if err := root.Rename(name, nativeStateName); err != nil {
		return err
	}
	return syncDirectory(root, ".")
}

func sameNativeManifest(a, b Manifest) bool {
	left, errA := json.Marshal(a)
	right, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(left) == string(right)
}
