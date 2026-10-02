// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"net/netip"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

type AcquisitionAttemptID struct {
	RegistrationID ddsnmp.DeviceRegistrationID
	Ordinal        uint64
}

type TargetResolutionOutcome uint8

const (
	TargetResolutionUnknown TargetResolutionOutcome = iota
	TargetResolutionLiteral
	TargetResolutionResolved
	TargetResolutionEmpty
	TargetResolutionUnavailable
	TargetResolutionFailed
)

type TargetResolutionEvidence struct {
	Outcome   TargetResolutionOutcome
	Addresses []netip.Addr
}

type AcquisitionPhaseOutcome uint8

const (
	AcquisitionPhaseUnknown AcquisitionPhaseOutcome = iota
	AcquisitionPhaseSuccess
	AcquisitionPhaseEmpty
	AcquisitionPhaseFailed
	AcquisitionPhaseNotObserved
)

type AcquisitionFailureClass uint8

const (
	AcquisitionFailureNone AcquisitionFailureClass = iota
	AcquisitionFailureClientConfiguration
	AcquisitionFailureConnect
	AcquisitionFailureCollection
	AcquisitionFailureSysUptime
	AcquisitionFailureVLANIdentifier
)

type AcquisitionPhaseEvidence struct {
	Detail  snmputils.Failure
	Outcome AcquisitionPhaseOutcome
	Failure AcquisitionFailureClass
}

type AcquisitionAttemptEvidence struct {
	Interruption       snmputils.Failure
	ProfileContext     *ddsnmp.ProfileContext
	VLANProfileContext *ddsnmp.ProfileContext
	ID                 AcquisitionAttemptID
	Device             DeviceInput
	Target             TargetResolutionEvidence
	Client             AcquisitionPhaseEvidence
	Connect            AcquisitionPhaseEvidence
	Profiles           AcquisitionPhaseEvidence
	Collection         AcquisitionPhaseEvidence
	SysUptime          AcquisitionPhaseEvidence
	VLANProfiles       AcquisitionPhaseEvidence
	CollectedAt        time.Time
	FreshFor           time.Duration
	SysUptimeValue     int64
	CollectionContexts []AcquisitionContextEvidence
}

type AcquisitionContextEvidence struct {
	Sources []*ddsnmp.SourceOperation

	Interruption snmputils.Failure
	Failures     ddsnmp.CollectionFailures
	Ordinal      uint32
	VLANID       string
	VLANName     string
	Client       AcquisitionPhaseEvidence
	Connect      AcquisitionPhaseEvidence
	Collection   AcquisitionPhaseEvidence
	Profiles     []AcquisitionProfileEvidence
}

type AcquisitionProfileEvidence struct {
	Identity     ddsnmpcollector.AcquisitionProfileIdentity
	Outcome      ddsnmpcollector.AcquisitionProfileOutcome
	FailurePhase ddsnmpcollector.AcquisitionFailurePhase
	Stats        ddsnmp.CollectionStats
	Execution    *ddsnmpcollector.AcquisitionExecutionReport
	Routes       []ddsnmpcollector.AcquisitionRouteReport
	Values       AcquisitionProfileValues
}

type AcquisitionCapture struct {
	AttemptID    AcquisitionAttemptID
	State        CaptureState
	Reason       CaptureReason
	RecordCount  uint64
	LogicalBytes uint64
	Evidence     *AcquisitionAttemptEvidence
}
