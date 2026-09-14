// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package secretresolver

import "os/exec"

func configureCommandProcessTree(*exec.Cmd) {}
