// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "errors"

var (
	errInvalidLabelKey      = errors.New("metrix: invalid label key")
	errInvalidLabelSet      = errors.New("metrix: invalid label set")
	errForeignLabelSet      = errors.New("metrix: foreign label set")
	errDuplicateLabelKey    = errors.New("metrix: duplicate label key")
	errVecLabelValueCount   = errors.New("metrix: vec label values count does not match label keys")
	errInvalidSampleValue   = errors.New("metrix: invalid sample value (NaN/Inf)")
	errCounterNegativeDelta = errors.New("metrix: counter Add delta cannot be negative")
	errCycleInactive        = errors.New("metrix: write outside active cycle")
	errCycleActive          = errors.New("metrix: cycle already active")
	errCycleMissing         = errors.New("metrix: cycle is not active")
	errHistogramLabelKey    = errors.New("metrix: histogram flatten label key collides with existing label")
	errHistogramSchema      = errors.New("metrix: histogram schema is missing")
	errHistogramBounds      = errors.New("metrix: histogram bounds are required")
	errHistogramPoint       = errors.New("metrix: invalid histogram point")
	errSummaryLabelKey      = errors.New("metrix: summary flatten label key collides with existing label")
	errSummaryPoint         = errors.New("metrix: invalid summary point")
	errStateSetSchema       = errors.New("metrix: stateset schema is missing")
	errStateSetEnumCount    = errors.New("metrix: stateset enum mode requires exactly one active state")
	errStateSetUnknownState = errors.New("metrix: stateset point contains undeclared state")
	errStateSetLabelKey     = errors.New("metrix: stateset flatten label key collides with existing label")
	errRuntimeSnapshotWrite = errors.New("metrix: runtime store supports stateful writes only")
	errRuntimeFreshness     = errors.New("metrix: runtime store freshness is fixed to FreshnessCommitted")
	errRuntimeWindowCycle   = errors.New("metrix: runtime store does not support window=cycle")
)
