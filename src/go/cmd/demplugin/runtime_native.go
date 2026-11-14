// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
)

const nativeProtocolLimit = 1 << 20

// runNativeCommand serves the installed package helper interface. These verbs
// take no runtime source or path overrides; package hooks own service
// stop/restart and transaction ordering.
func runNativeCommand(ctx context.Context, command string, args []string, stdout, stderr io.Writer) error {
	root := false
	switch command {
	case "native-prepare", "native-configure":
		root = true
	case "native-initialize", "native-inspect", "native-qualify":
	default:
		return fmt.Errorf("unknown internal native runtime command %q", command)
	}
	if len(args) != 0 {
		return errors.New("internal native runtime commands accept no flags or positional arguments")
	}
	if err := nativeCommandPlatform(); err != nil {
		return err
	}
	if root && os.Geteuid() != 0 {
		return errors.New("native package preparation/configuration requires root")
	}
	if !root && os.Geteuid() == 0 {
		return errors.New("native proof and qualification commands must run as the configured service UID")
	}
	if root {
		return runNativeRootCommand(ctx, command, stdout, stderr)
	}
	return runNativeServiceCommand(ctx, command, os.Stdin, stdout)
}

func runNativeServiceCommand(ctx context.Context, command string, stdin io.Reader, stdout io.Writer) error {
	config := runtime.DefaultNativeConfig()
	if command == "native-initialize" {
		if err := runtime.InitializeNativeProof(config); err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(nativeInitialized{Initialized: true})
	}
	var target runtime.Manifest
	if command == "native-qualify" {
		if err := decodeNativeProtocol(stdin, &target); err != nil {
			return err
		}
		if err := target.Validate(); err != nil {
			return err
		}
	}
	native, err := runtime.OpenNative(config)
	if err != nil {
		return err
	}
	if command == "native-inspect" {
		err = inspectNative(native, stdout)
	} else {
		err = qualifyNative(ctx, native, target, stdout)
	}
	// An undrained execution retains its proof and work. Process exit releases
	// local locks; the persistent receipt remains the authority for recovery.
	return closeUnlessUndrained(err, native)
}

func inspectNative(native *runtime.Native, stdout io.Writer) error {
	if err := native.Sweep(); err != nil {
		return err
	}
	status, err := native.Inspect()
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(status)
}

func qualifyNative(ctx context.Context, native *runtime.Native, target runtime.Manifest, stdout io.Writer) error {
	candidate, err := native.AcquireCandidate(target)
	if err != nil {
		return err
	}
	qualification, err := runner.Qualify(ctx, runtimehost.Borrow(candidate))
	if err == nil {
		err = json.NewEncoder(stdout).Encode(qualification)
	}
	return closeUnlessUndrained(err, candidate)
}

func runNativeRootCommand(ctx context.Context, command string, stdout, stderr io.Writer) error {
	identity, err := nativeServiceAccount()
	if err != nil {
		return err
	}
	child, err := newNativeChild(identity, stderr)
	if err != nil {
		return err
	}
	config := runtime.DefaultNativeConfig()
	inspect := func(ctx context.Context) (runtime.NativeStatus, error) {
		data, err := child.run(ctx, "native-inspect", nil)
		if err != nil {
			return runtime.NativeStatus{}, err
		}
		return decodeNativeStatus(data)
	}
	if command == "native-prepare" {
		return closeNativeAdmission(ctx, config, identity, child, inspect, stdout)
	}
	return configureNative(ctx, config, child, inspect, stdout)
}

// nativeInitialized is the service child's native-initialize reply.
type nativeInitialized struct {
	Initialized bool `json:"initialized"`
}

// nativePhase is the root helper's result for package scriptlets.
type nativePhase struct {
	Phase string `json:"phase"`
}

// closeNativeAdmission initializes the service proof through the service child
// and closes package admission.
func closeNativeAdmission(ctx context.Context, config runtime.NativeConfig, identity nativeIdentity, child *nativeChild, inspect runtime.NativeInspector, stdout io.Writer) error {
	control, err := runtime.InitializeNativeControl(ctx, config, int(identity.uid), int(identity.gid), func(ctx context.Context) error {
		data, err := child.run(ctx, "native-initialize", nil)
		if err != nil {
			return err
		}
		var reply nativeInitialized
		if err := decodeNativeProtocol(bytes.NewReader(data), &reply); err != nil {
			return err
		}
		if !reply.Initialized {
			return errors.New("native service proof initialization was not confirmed")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := control.CloseAdmission(ctx, inspect); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(nativePhase{Phase: "closed"})
}

// configureNative qualifies the admitted target as the service UID and marks
// the native runtime ready.
func configureNative(ctx context.Context, config runtime.NativeConfig, child *nativeChild, inspect runtime.NativeInspector, stdout io.Writer) error {
	control, err := runtime.OpenNativeControl(config)
	if err != nil {
		return err
	}
	if err := control.QualifyAndReady(ctx, inspect, func(ctx context.Context, target runtime.Manifest) error {
		input, err := json.Marshal(target)
		if err != nil {
			return err
		}
		data, err := child.run(ctx, "native-qualify", input)
		if err != nil {
			return err
		}
		return verifyNativeQualification(data, target)
	}); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(nativePhase{Phase: "ready"})
}

func decodeNativeProtocol(reader io.Reader, value any) error {
	data, err := io.ReadAll(io.LimitReader(reader, nativeProtocolLimit+1))
	if err != nil {
		return err
	}
	if len(data) > nativeProtocolLimit {
		return errors.New("native helper protocol exceeds 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode native helper protocol: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("native helper protocol contains trailing data")
	}
	return nil
}

func decodeNativeStatus(data []byte) (runtime.NativeStatus, error) {
	var status runtime.NativeStatus
	if err := decodeNativeProtocol(bytes.NewReader(data), &status); err != nil {
		return status, err
	}
	// Missing/null counters cannot be interpreted as a successful empty proof.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return status, err
	}
	for _, name := range []string{"process_claims", "candidate_claims", "execution_pins", "uncertain_pins"} {
		value, exists := fields[name]
		if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return status, fmt.Errorf("native inspector omitted %s", name)
		}
	}
	return status, nil
}

func verifyNativeQualification(data []byte, target runtime.Manifest) error {
	var qualification runner.Qualification
	if err := decodeNativeProtocol(bytes.NewReader(data), &qualification); err != nil {
		return err
	}
	if !qualification.ExecutionContract || !qualification.SandboxedLocalJourney || !qualification.SandboxedLocalLighthouse || !qualification.ProcessTreeDrained || qualification.Runtime == nil {
		return errors.New("native service did not confirm complete, drained qualification")
	}
	if *qualification.Runtime != target.Identity() {
		return errors.New("native qualification identity differs from admitted target")
	}
	return nil
}
