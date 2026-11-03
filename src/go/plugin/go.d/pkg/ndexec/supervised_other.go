// SPDX-License-Identifier: GPL-3.0-or-later
//go:build !linux

package ndexec

import "context"

func startSupervisedNDSudo(_ context.Context, _ ProcessOptions, _, _ string, lease *acquisitionLease) (*Process, error) {
	lease.release()
	return nil, ErrSupervisedUnsupported
}
