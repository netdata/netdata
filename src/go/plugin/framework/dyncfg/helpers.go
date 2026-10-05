// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"context"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

// CommandFromArgs returns the case-normalized command in Args[1], or an empty
// command when the argument is missing.
func CommandFromArgs(args []string) Command {
	if len(args) < 2 {
		return ""
	}
	return Command(strings.ToLower(args[1]))
}

// WrapHandler adapts a dyncfg function handler to functions.Registry handler type.
func WrapHandler(handler func(Function)) functions.Handler {
	return func(ctx context.Context, fn functions.Function) {
		handler(NewFunction(ctx, fn))
	}
}
