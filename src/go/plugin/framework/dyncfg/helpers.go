// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

var jobNameReplacer = strings.NewReplacer(" ", "_", ":", "_")

// CommandFromArgs returns the case-normalized command in Args[1], or an empty
// command when the argument is missing.
func CommandFromArgs(args []string) Command {
	if len(args) < 2 {
		return ""
	}
	return Command(strings.ToLower(args[1]))
}

// NormalizeJobName replaces spaces and colons with underscores without
// otherwise changing the name.
func NormalizeJobName(name string) string {
	return jobNameReplacer.Replace(name)
}

// WrapHandler adapts a dyncfg function handler to functions.Registry handler type.
func WrapHandler(handler func(Function)) func(functions.Function) {
	return func(fn functions.Function) {
		handler(NewFunction(fn))
	}
}
