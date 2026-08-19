//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris

package runner

import "os/exec"

func configureContainment(_ *exec.Cmd) {}

func killContained(command *exec.Cmd) error {
	// These platforms have no shared process-group implementation here, so the
	// fallback contains only the direct child process.
	if command.Process == nil {
		return nil
	}
	return command.Process.Kill()
}
