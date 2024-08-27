// SPDX-License-Identifier: GPL-3.0-or-later

package runit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

// DefaultDir returns default service directory in a way used by runit sv(8) tool.
func defaultDir() string {
	if dir := os.Getenv("SVDIR"); dir != "" {
		return dir
	}
	return "/service"
}

func (s Runit) validateConfig() error {
	if s.Dir == "" {
		return errors.New("'path' option not set")
	}
	s.Dir = filepath.Clean(s.Dir)

	switch fi, err := os.Stat(s.Dir); {
	case err != nil:
		return fmt.Errorf("'path' invalid: %w", err)
	case !fi.IsDir():
		return errors.New("'path' must be a directory")
	}

	return nil
}

func (s *Runit) initSvCli() error {
	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")
	if _, err := os.Stat(ndsudoPath); err != nil {
		return fmt.Errorf("ndsudo executable not found: %w", err)
	}

	s.exec = newSvCliExec(ndsudoPath, s.execTimeout, s.Logger)

	return nil
}
