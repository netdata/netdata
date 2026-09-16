// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux && !darwin

package notifier

import (
	"errors"
	"os/exec"
)

func configureCommandProcess(_ *exec.Cmd) error {
	return errors.New("command delivery is currently supported only on Linux and macOS")
}
