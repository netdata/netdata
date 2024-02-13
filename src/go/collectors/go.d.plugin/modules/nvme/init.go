// SPDX-License-Identifier: GPL-3.0-or-later

package nvme

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func (n *NVMe) validateConfig() error {
	if n.BinaryPath == "" {
		return errors.New("'binary_path' can not be empty")
	}

	return nil
}

func (n *NVMe) initNVMeCLIExec() (nvmeCLI, error) {
	if exePath, err := os.Executable(); err == nil {
		ndsudoPath := filepath.Join(filepath.Dir(exePath), "ndsudo")

		if fi, err := os.Stat(ndsudoPath); err == nil {
			// executable by owner or group
			if fi.Mode().Perm()&0110 != 0 {
				n.Debug("using ndsudo")
				return &nvmeCLIExec{
					ndsudoPath: ndsudoPath,
					timeout:    n.Timeout.Duration,
				}, nil
			}
		}
	}

	// TODO: remove after next minor release of Netdata (latest is v1.44.0)
	// can't remove now because it will break "from source + stable channel" installations
	nvmePath, err := exec.LookPath(n.BinaryPath)
	if err != nil {
		return nil, err
	}

	var sudoPath string
	if os.Getuid() != 0 {
		sudoPath, err = exec.LookPath("sudo")
		if err != nil {
			return nil, err
		}
	}

	if sudoPath != "" {
		ctx1, cancel1 := context.WithTimeout(context.Background(), n.Timeout.Duration)
		defer cancel1()

		if _, err := exec.CommandContext(ctx1, sudoPath, "-n", "-v").Output(); err != nil {
			return nil, fmt.Errorf("can not run sudo on this host: %v", err)
		}

		ctx2, cancel2 := context.WithTimeout(context.Background(), n.Timeout.Duration)
		defer cancel2()

		if _, err := exec.CommandContext(ctx2, sudoPath, "-n", "-l", nvmePath).Output(); err != nil {
			return nil, fmt.Errorf("can not run '%s' with sudo: %v", n.BinaryPath, err)
		}
	}

	return &nvmeCLIExec{
		sudoPath: sudoPath,
		nvmePath: nvmePath,
		timeout:  n.Timeout.Duration,
	}, nil
}
