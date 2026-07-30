// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"context"
	"errors"
	"strings"
)

// Status represents the state of a dyncfg entity
type Status string

const (
	StatusAccepted   Status = "accepted"
	StatusRunning    Status = "running"
	StatusFailed     Status = "failed"
	StatusIncomplete Status = "incomplete"
	StatusDisabled   Status = "disabled"
)

func (s Status) String() string {
	return string(s)
}

type ConfigType string

const (
	ConfigTypeSingle   ConfigType = "single"
	ConfigTypeTemplate ConfigType = "template"
	ConfigTypeJob      ConfigType = "job"
)

func (t ConfigType) String() string {
	return string(t)
}

type Command string

const (
	CommandAdd        Command = "add"
	CommandRemove     Command = "remove"
	CommandGet        Command = "get"
	CommandUpdate     Command = "update"
	CommandRestart    Command = "restart"
	CommandEnable     Command = "enable"
	CommandDisable    Command = "disable"
	CommandTest       Command = "test"
	CommandSchema     Command = "schema"
	CommandUserconfig Command = "userconfig"
)

// Testable is an optional operational-test capability for configured resources.
// Implementations must honor ctx and release resources acquired by the test.
// They must return failures for caller-side sanitization instead of logging raw
// user configuration, credentials, or endpoint material. NewPublicError may
// attach a static, code-authored explanation that is safe to render publicly.
type Testable interface {
	Test(ctx context.Context) error
}

func JoinCommands(commands ...Command) string {
	strs := make([]string, len(commands))
	for i, cmd := range commands {
		strs[i] = string(cmd)
	}
	return strings.Join(strs, " ")
}

// ErrNonDisruptiveUpdate marks update failures where runtime state was not changed.
// Handler rollback logic uses this marker to keep old config/status authoritative.
var ErrNonDisruptiveUpdate = errors.New("non-disruptive update")

type nonDisruptiveUpdateError struct {
	err error
}

func (e *nonDisruptiveUpdateError) Error() string        { return e.err.Error() }
func (e *nonDisruptiveUpdateError) Unwrap() error        { return e.err }
func (e *nonDisruptiveUpdateError) Is(target error) bool { return target == ErrNonDisruptiveUpdate }

// MarkNonDisruptiveUpdate wraps err to indicate update failed before disrupting runtime.
func MarkNonDisruptiveUpdate(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, ErrNonDisruptiveUpdate) {
		return err
	}

	return &nonDisruptiveUpdateError{err: err}
}
