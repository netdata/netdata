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
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

const nativeProtocolLimit = 1 << 20
const nativeDiagnosticLimit = 16 << 10

// These verbs are the installed package helper interface, not runtime source or
// path overrides. Package hooks own service stop/restart and transaction ordering.
func runNativeRuntimeCommand(ctx context.Context, args []string, stdout, stderr io.Writer) (bool, error) {
	if len(args) < 2 || args[0] != "runtime" || !strings.HasPrefix(args[1], "native-") {
		return false, nil
	}
	command := args[1]
	root := false
	switch command {
	case "native-prepare", "native-configure":
		root = true
	case "native-initialize", "native-inspect", "native-qualify":
	default:
		return true, fmt.Errorf("unknown internal native runtime command %q", command)
	}
	if len(args) != 2 {
		return true, errors.New("internal native runtime commands accept no flags or positional arguments")
	}
	if err := nativeCommandPlatform(); err != nil {
		return true, err
	}
	if root && os.Geteuid() != 0 {
		return true, errors.New("native package preparation/configuration requires root")
	}
	if !root && os.Geteuid() == 0 {
		return true, errors.New("native proof and qualification commands must run as the configured service UID")
	}
	if root {
		return true, runNativeRootCommand(ctx, command, stdout, stderr)
	}
	return true, runNativeServiceCommand(ctx, command, os.Stdin, stdout)
}

func runNativeServiceCommand(ctx context.Context, command string, stdin io.Reader, stdout io.Writer) (result error) {
	config := runtime.DefaultNativeConfig()
	if command == "native-initialize" {
		if err := runtime.InitializeNativeProof(config); err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(struct {
			Initialized bool `json:"initialized"`
		}{true})
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
	defer func() {
		// An undrained execution retains its proof and work. Process exit releases
		// local locks; the persistent receipt remains the authority for recovery.
		if !errors.Is(result, ndexec.ErrTreeNotDrained) {
			result = errors.Join(result, native.Close())
		}
	}()
	if command == "native-inspect" {
		if err := native.Sweep(); err != nil {
			return err
		}
		status, err := native.Inspect()
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(status)
	}
	candidate, err := native.AcquireCandidate(target)
	if err != nil {
		return err
	}
	defer func() {
		if !errors.Is(result, ndexec.ErrTreeNotDrained) {
			result = errors.Join(result, candidate.Close())
		}
	}()
	source := runtimehost.Borrow(candidate)
	qualification, err := runner.New(source, nil, nil).Qualify(ctx, source)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(qualification)
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
		control, err := runtime.InitializeNativeControl(ctx, config, int(identity.uid), int(identity.gid), func(ctx context.Context) error {
			data, err := child.run(ctx, "native-initialize", nil)
			if err != nil {
				return err
			}
			var reply struct {
				Initialized bool `json:"initialized"`
			}
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
		return json.NewEncoder(stdout).Encode(struct {
			Phase string `json:"phase"`
		}{"closed"})
	}
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
	return json.NewEncoder(stdout).Encode(struct {
		Phase string `json:"phase"`
	}{"ready"})
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
	identity := qualification.Runtime
	if identity.Version != target.Version || identity.SHA256 != target.Archive.SHA256 || identity.Contract != target.Contract {
		return errors.New("native qualification identity differs from admitted target")
	}
	return nil
}
