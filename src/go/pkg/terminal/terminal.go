// SPDX-License-Identifier: GPL-3.0-or-later

package terminal

import (
	"os"

	"github.com/mattn/go-isatty"
)

// IsTerminal reports whether plugin IO is attached to a terminal.
// It checks stderr, stdout, and stdin.
func IsTerminal() bool {
	return isatty.IsTerminal(os.Stderr.Fd()) ||
		isatty.IsTerminal(os.Stdout.Fd()) ||
		isatty.IsTerminal(os.Stdin.Fd())
}
