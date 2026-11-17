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

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/browserruntime"
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
	case "native-status":
		root = os.Geteuid() == 0
	case "native-initialize", "native-inspect", "native-validate":
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
		return errors.New("native proof and validation commands must run as the configured service UID")
	}
	if root {
		return runNativeRootCommand(ctx, command, stdout, stderr)
	}
	return runNativeServiceCommand(ctx, command, os.Stdin, stdout)
}

func runNativeServiceCommand(ctx context.Context, command string, stdin io.Reader, stdout io.Writer) error {
	config := runtimehost.NativeConfig()
	if command == "native-initialize" {
		if err := browserruntime.InitializeNativeProof(config); err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(nativeInitialized{
			Initialized: true,
		})
	}
	var target browserruntime.Manifest
	if command == "native-validate" {
		if err := decodeNativeProtocol(stdin, &target); err != nil {
			return err
		}
		if err := target.Validate(); err != nil {
			return err
		}
	}
	native, err := browserruntime.OpenNative(config)
	if err != nil {
		return err
	}
	if command == "native-status" {
		err = reportNativeAdmission(native, stdout)
	} else if command == "native-inspect" {
		err = inspectNative(native, stdout)
	} else {
		err = validateNative(ctx, native, target, stdout)
	}
	return errors.Join(err, native.Close())
}

// The updater verifies exact pairing through the service-account admission path.
// Reading ownership counters alone cannot establish that a posttrans succeeded.
func reportNativeAdmission(native *browserruntime.Native, stdout io.Writer) error {
	consumer, err := native.AcquireSelectedPair()
	if err != nil {
		return err
	}
	identity := consumer.Manifest().Identity()
	if err := consumer.Close(); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(nativeValidation{Validated: true, Runtime: &identity})
}

func inspectNative(native *browserruntime.Native, stdout io.Writer) error {
	if err := native.Sweep(); err != nil {
		return err
	}
	status, err := native.Inspect()
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(status)
}

// validateNative acquires the admitted payload through the service reader. Its
// complete identity and integrity checks run without starting Node or Chromium.
func validateNative(
	ctx context.Context,
	native *browserruntime.Native,
	target browserruntime.Manifest,
	stdout io.Writer,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	candidate, err := native.AcquireCandidate(target)
	if err != nil {
		return err
	}
	identity := candidate.Manifest().Identity()
	if err := candidate.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(nativeValidation{
		Validated: true,
		Runtime:   &identity,
	})
}

// nativeValidation acknowledges integrity validation, never browser qualification.
type nativeValidation struct {
	Validated bool                       `json:"validated"`
	Runtime   *synthetic.RuntimeIdentity `json:"runtime"`
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
	if command == "native-status" {
		data, err := child.run(ctx, command, nil)
		if err != nil {
			return err
		}
		var status nativeValidation
		if err := decodeNativeProtocol(bytes.NewReader(data), &status); err != nil {
			return err
		}
		if !status.Validated || status.Runtime == nil {
			return errors.New("native service did not confirm runtime admission")
		}
		return json.NewEncoder(stdout).Encode(status)
	}
	config := runtimehost.NativeConfig()
	inspect := func(ctx context.Context) (browserruntime.NativeStatus, error) {
		data, err := child.run(ctx, "native-inspect", nil)
		if err != nil {
			return browserruntime.NativeStatus{}, err
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
func closeNativeAdmission(
	ctx context.Context,
	config browserruntime.NativeConfig,
	identity nativeIdentity,
	child *nativeChild,
	inspect browserruntime.NativeInspector,
	stdout io.Writer,
) error {
	control, err := browserruntime.InitializeNativeControl(
		ctx,
		config,
		int(identity.uid),
		int(identity.gid),
		func(ctx context.Context) error {
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
		},
	)
	if err != nil {
		return err
	}
	if err := control.CloseAdmission(ctx, inspect); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(nativePhase{
		Phase: "closed",
	})
}

// configureNative validates the admitted target as the service UID and marks
// the native runtime ready.
func configureNative(
	ctx context.Context,
	config browserruntime.NativeConfig,
	child *nativeChild,
	inspect browserruntime.NativeInspector,
	stdout io.Writer,
) error {
	control, err := browserruntime.OpenNativeControl(config)
	if err != nil {
		return err
	}
	if err := control.ValidateAndReady(ctx, inspect, func(ctx context.Context, target browserruntime.Manifest) error {
		input, err := json.Marshal(target)
		if err != nil {
			return err
		}
		data, err := child.run(ctx, "native-validate", input)
		if err != nil {
			return err
		}
		return verifyNativeValidation(data, target)
	}); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(nativePhase{
		Phase: "ready",
	})
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

func decodeNativeStatus(data []byte) (browserruntime.NativeStatus, error) {
	var status browserruntime.NativeStatus
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

func verifyNativeValidation(data []byte, target browserruntime.Manifest) error {
	var validation nativeValidation
	if err := decodeNativeProtocol(bytes.NewReader(data), &validation); err != nil {
		return err
	}
	if !validation.Validated || validation.Runtime == nil {
		return errors.New("native service did not confirm payload integrity validation")
	}
	if *validation.Runtime != target.Identity() {
		return errors.New("native validation identity differs from admitted target")
	}
	return nil
}
