// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"
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

func JoinCommands(commands ...Command) string {
	strs := make([]string, len(commands))
	for i, cmd := range commands {
		strs[i] = string(cmd)
	}
	return strings.Join(strs, " ")
}

// GetCommand extracts the dyncfg command from a function's arguments.
// Returns empty string if args has fewer than 2 elements.
func GetCommand(fn functions.Function) Command {
	if len(fn.Args) < 2 {
		return ""
	}
	return Command(strings.ToLower(fn.Args[1]))
}

// GetSourceValue extracts a value from the function's Source field.
// Source format is "key1=value1,key2=value2,...".
func GetSourceValue(fn functions.Function, key string) string {
	prefix := key + "="
	for _, part := range strings.Split(fn.Source, ",") {
		if v, ok := strings.CutPrefix(part, prefix); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
