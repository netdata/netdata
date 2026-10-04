// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package ndexec

import "context"

// StartUnprivilegedProcessTree has no signal-only or uncontained fallback on
// platforms without the supported Linux supervisor.
func StartUnprivilegedProcessTree(
	ctx context.Context,
	opts ProcessOptions,
	binPath string,
	args ...string,
) (*TreeProcess, error) {
	return nil, ErrTreeSupervisionUnsupported
}
