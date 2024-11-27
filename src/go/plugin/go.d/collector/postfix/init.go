// SPDX-License-Identifier: GPL-3.0-or-later

package postfix

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

func (c *Collector) validateConfig() error {
	if c.BinaryPath == "" {
		return errors.New("no postqueue binary path specified")
	}
	return nil
}

func (c *Collector) initPostqueueExec() (postqueueBinary, error) {
	binPath := c.BinaryPath

	if !strings.HasPrefix(binPath, "/") {
		path, err := exec.LookPath(binPath)
		if err != nil {
			return nil, err
		}
		binPath = path
	}

	if _, err := os.Stat(binPath); err != nil {
		return nil, err
	}

	pq := newPostqueueExec(binPath, c.Timeout.Duration())
	pq.Logger = c.Logger

	return pq, nil
}
