// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package secretresolver

import (
	"context"
	"os/exec"
)

func secretCommandContext(ctx context.Context, binPath string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, binPath, args...)
}
