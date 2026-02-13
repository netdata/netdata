// SPDX-License-Identifier: GPL-3.0-or-later

package storcli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

func (c *Collector) initStorCliExec() (storCli, error) {
	if runtime.GOOS == "windows" {
		return c.initDirectStorCliExec()
	}
	return c.initNdsudoStorCliExec()
}

func (c *Collector) initNdsudoStorCliExec() (storCli, error) {
	storExec := newNdsudoStorCliExec(c.Timeout.Duration(), c.Logger)
	return storExec, nil
}

func (c *Collector) initDirectStorCliExec() (storCli, error) {
	path, err := ndexec.FindBinary(
		[]string{"storcli", "storcli64", "StorCLI", "StorCLI64"},
		[]string{
			filepath.Join(os.Getenv("ProgramFiles"), "Broadcom", "StorCLI", "storcli64.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "LSI", "StorCLI", "storcli64.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "StorCLI", "storcli64.exe"),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("storcli: %w", err)
	}

	c.Debugf("found storcli at: %s", path)

	return newDirectStorCliExec(path, c.Timeout.Duration(), c.Logger), nil
}
