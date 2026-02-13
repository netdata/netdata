// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "errors"

var (
	errInvalidLabelKey      = errors.New("metrix: invalid label key")
	errInvalidLabelSet      = errors.New("metrix: invalid label set")
	errForeignLabelSet      = errors.New("metrix: foreign label set")
	errDuplicateLabelKey    = errors.New("metrix: duplicate label key")
	errCounterNegativeDelta = errors.New("metrix: counter Add delta cannot be negative")
	errCycleInactive        = errors.New("metrix: write outside active cycle")
	errCycleActive          = errors.New("metrix: cycle already active")
	errCycleMissing         = errors.New("metrix: cycle is not active")
	errStateSetSchema       = errors.New("metrix: stateset schema is missing")
	errStateSetEnumCount    = errors.New("metrix: stateset enum mode requires exactly one active state")
	errStateSetUnknownState = errors.New("metrix: stateset point contains undeclared state")
	errStateSetLabelKey     = errors.New("metrix: stateset flatten label key collides with existing label")
)
