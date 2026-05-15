// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package ndexec

import "os/exec"

func configureCommandCancellation(cmd *exec.Cmd) {}
