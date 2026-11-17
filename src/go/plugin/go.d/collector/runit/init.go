// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package runit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// defaultDir returns default service directory in a way used by runit sv(8) tool.
func defaultDir() string {
	if dir := os.Getenv("SVDIR"); dir != "" {
		return dir
	}
	return "/service"
}

func (c *Collector) validateConfig() error {
	if c.Dir == "" {
		return errors.New("'dir' option not set")
	}
	c.Dir = filepath.Clean(c.Dir)

	switch fi, err := os.Stat(c.Dir); {
	case err != nil:
		return fmt.Errorf("'dir' invalid: %w", err)
	case !fi.IsDir():
		return errors.New("'dir' must be a directory")
	}

	return nil
}

func (c *Collector) initSvCli() error {
	c.exec = &svCliExec{
		timeout: time.Second,
		log:     c.Logger,
	}
	return nil
}
