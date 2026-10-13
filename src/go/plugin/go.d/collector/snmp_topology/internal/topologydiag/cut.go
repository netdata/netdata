// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

type LifecycleCut struct {
	State  CaptureState
	Reason CaptureReason
	Cut    ddsnmp.DeviceLifecycleCut
}

type SweepDevice struct {
	RegistrationID ddsnmp.DeviceRegistrationID
	Selected       bool
	Outcome        RefreshOutcome
	LastAttempt    time.Time
	LastSuccess    time.Time
	NextRetry      time.Time

	RetainedSuccess    EvidenceRef
	HasRetainedSuccess bool
	Acquisition        *AcquisitionCapture
	LatestAttempt      *AcquisitionCapture
	HasObservation     bool
	ExpiresAt          time.Time
	Renderable         bool
	Expired            bool
}

type RemovedDevice struct {
	RegistrationID     ddsnmp.DeviceRegistrationID
	RetainedSuccess    EvidenceRef
	HasRetainedSuccess bool
}

type SweepCut struct {
	Sequence      uint64
	StartedAt     time.Time
	PublishedAt   time.Time
	CaptureState  CaptureState
	CaptureReason CaptureReason
	RecordCount   uint64
	LogicalBytes  uint64
	Devices       []SweepDevice
	Removed       []RemovedDevice
}

type DiagnosticAbortReason uint8

const (
	DiagnosticAbortUnknown DiagnosticAbortReason = iota
	DiagnosticAbortCanceled
	DiagnosticAbortPanic
)

type DiagnosticSweepPhase uint8

const (
	DiagnosticSweepPhaseUnknown DiagnosticSweepPhase = iota
	DiagnosticSweepPhaseRegistrationCut
	DiagnosticSweepPhaseTargetResolution
	DiagnosticSweepPhaseDeviceRefresh
	DiagnosticSweepPhaseCommit
)

type AbortedSweep struct {
	Sequence              uint64
	StartedAt             time.Time
	AbortedAt             time.Time
	Reason                DiagnosticAbortReason
	Phase                 DiagnosticSweepPhase
	ActiveRegistrationID  ddsnmp.DeviceRegistrationID
	HasActiveRegistration bool
	RegistrationCount     int
	SelectedCount         int
}

type Cut struct {
	Lifecycle       LifecycleCut
	ProducerScopeID string
	Topology        *SweepCut
	LastAborted     *AbortedSweep
}

type RefreshOutcome uint8

const (
	RefreshOutcomeUnknown RefreshOutcome = iota
	RefreshOutcomeSuccess
	RefreshOutcomeNoProfiles
	RefreshOutcomeFailed
)

type EvidenceRef struct {
	RegistrationID ddsnmp.DeviceRegistrationID
	Generation     uint64
}
