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
// Explicit collector tests call Test after Init, instead of Check. Collectors
// without this capability use Check; activation always uses Init and Check.
// Implementations must honor ctx and release resources acquired by the test.
// They must return failures for caller-specific error handling instead of logging raw
// user configuration, credentials, or endpoint material. NewPublicError may
// attach a static, code-authored explanation that is safe to render publicly.
// Collectors must return response-safe error text: without secret references,
// their lifecycle errors are returned as-is. With secret references, collector
// sanitization replaces error text, including NewPublicError explanations, and
// does not preserve ErrTestUnsupported identity. Either case remains a failure.
// Test may return ErrTestUnsupported when a valid configured instance cannot
// safely perform an operational test.
type Testable interface {
	Test(ctx context.Context) error
}

// ErrTestUnsupported reports that a Testable resource cannot safely run an
// operational test for the configured instance.
var ErrTestUnsupported = errors.New("operational test is not supported")

func JoinCommands(commands ...Command) string {
	strs := make([]string, len(commands))
	for i, cmd := range commands {
		strs[i] = string(cmd)
	}
	return strings.Join(strs, " ")
}
