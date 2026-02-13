// SPDX-License-Identifier: GPL-3.0-or-later

package hpssa

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

func (c *Collector) initSsacliBinary() (ssacliBinary, error) {
	if runtime.GOOS == "windows" {
		return c.initDirectSsacliExec()
	}
	return c.initNdsudoSsacliExec()
}

func (c *Collector) initNdsudoSsacliExec() (ssacliBinary, error) {
	ssacliExec := newNdsudoSsacliExec(c.Timeout.Duration(), c.Logger)
	return ssacliExec, nil
}

func (c *Collector) initDirectSsacliExec() (ssacliBinary, error) {
	path, err := ndexec.FindBinary(
		[]string{"ssacli", "SSACLI", "hpssacli"},
		[]string{
			filepath.Join(os.Getenv("ProgramFiles"), "Smart Storage Administrator", "ssacli", "bin", "ssacli.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "HP", "hpssacli", "bin", "hpssacli.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Compaq", "Hpacucli", "Bin", "hpacucli.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Smart Storage Administrator", "ssacli", "bin", "ssacli.exe"),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("ssacli: %w", err)
	}

	c.Debugf("found ssacli at: %s", path)

	return newDirectSsacliExec(path, c.Timeout.Duration(), c.Logger), nil
}
