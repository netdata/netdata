// SPDX-License-Identifier: GPL-3.0-or-later

package postfix

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

func (p *Postfix) validateConfig() error {
	if p.BinaryPath == "" {
		return errors.New("no postqueue binary path specified")
	}
	return nil
}

func (p *Postfix) initPostqueueExec() (postqueueBinary, error) {
	binPath := p.BinaryPath

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

	pq := newPostqueueExec(binPath, p.Timeout.Duration())
	pq.Logger = p.Logger

	return pq, nil
}
