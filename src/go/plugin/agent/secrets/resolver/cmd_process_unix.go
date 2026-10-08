// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package secretresolver

import (
	"context"
	"os/exec"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

func secretCommandContext(ctx context.Context, binPath string, args ...string) *exec.Cmd {
	return ndexec.UnprivilegedCommandContextWithPreservedEnv(ctx, binPath, args...)
}
