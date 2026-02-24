// SPDX-License-Identifier: GPL-3.0-or-later

package terminal

import (
	"os"

	"github.com/mattn/go-isatty"
)

// IsTerminal reports whether plugin IO is attached to a terminal.
// It checks stdout and stdin for consistent behavior across components.
func IsTerminal() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsTerminal(os.Stdin.Fd())
}
