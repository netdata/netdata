// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux && !darwin && !freebsd && !windows

package ndexec

import "fmt"

func startOwnedProcess(string, []string, ProcessOptions) (processHandle, error) {
	return nil, fmt.Errorf("owned process containment is unsupported on this platform")
}
