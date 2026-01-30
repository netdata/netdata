// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
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
