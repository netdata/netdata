//go:build !linux && !darwin && !freebsd

// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"errors"
	"time"
)

func monotonicNow() (time.Duration, error) {
	return 0, errors.New("journey monotonic clock is unsupported on this platform")
}
