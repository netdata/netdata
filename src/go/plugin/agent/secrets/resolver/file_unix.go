// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package secretresolver

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

func readSecretFile(ctx context.Context, path string) ([]byte, error) {
	// Pass the absolute path as one argument: no shell and no privileged file open.
	return runSecretCommand(ndexec.UnprivilegedCommandContext(ctx, "/bin/cat", path))
}
