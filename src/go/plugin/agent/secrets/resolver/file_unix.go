// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package secretresolver

import (
	"context"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

const defaultFileTimeout = 3 * time.Second

func readSecretFile(ctx context.Context, path string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, defaultFileTimeout)
	defer cancel()

	// Pass the absolute path as one argument: no shell and no privileged file open.
	out, err := runSecretCommand(ndexec.UnprivilegedCommandContext(ctx, "/bin/cat", path))
	if err != nil {
		return nil, errors.Join(ctx.Err(), err)
	}
	return out, nil
}
