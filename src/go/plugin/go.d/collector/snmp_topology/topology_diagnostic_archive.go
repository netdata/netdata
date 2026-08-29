// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

const (
	topologyDiagnosticArchiveFormat  = "netdata.snmp_topology.diagnostics"
	topologyDiagnosticArchiveVersion = 1

	// The defaults leave substantial headroom over the largest measured producer shapes.
	topologyDiagnosticArchiveDefaultMaxCompressedBytes = 128 << 20
	topologyDiagnosticArchiveDefaultMaxDecodedBytes    = 512 << 20
)

var (
	errTopologyDiagnosticArchiveCompressedLimit = errors.New("SNMP topology diagnostic archive compressed-byte limit exceeded")
	errTopologyDiagnosticArchiveDecodedLimit    = errors.New("SNMP topology diagnostic archive decoded-byte limit exceeded")
)

type topologyDiagnosticArchiveReadLimits struct {
	maxCompressedBytes int64
	maxDecodedBytes    int64
}

func defaultTopologyDiagnosticArchiveReadLimits() topologyDiagnosticArchiveReadLimits {
	return topologyDiagnosticArchiveReadLimits{
		maxCompressedBytes: topologyDiagnosticArchiveDefaultMaxCompressedBytes,
		maxDecodedBytes:    topologyDiagnosticArchiveDefaultMaxDecodedBytes,
	}
}

var topologyDiagnosticArchiveWriterJSONOptions = jsonv2.JoinOptions(
	jsonv1.DefaultOptionsV1(),
	jsontext.EscapeForHTML(false),
)

var topologyDiagnosticArchiveReaderJSONOptions = jsonv2.JoinOptions(
	jsonv1.DefaultOptionsV1(),
	jsontext.AllowInvalidUTF8(false),
)

type topologyDiagnosticArchive struct {
	producerVersion string
	diagnostics     topologyDiagnostics
}

type topologyDiagnosticArchiveDocumentV1 struct {
	Format   string                              `json:"format"`
	Version  uint64                              `json:"version"`
	Producer topologyDiagnosticArchiveProducerV1 `json:"producer"`
	Snapshot topologyDiagnosticArchiveSnapshotV1 `json:"snapshot"`
}

type topologyDiagnosticArchiveProducerV1 struct {
	AgentVersion string `json:"agent_version"`
}

type topologyDiagnosticArchiveSnapshotV1 struct {
	Lifecycle       topologyDiagnosticArchiveLifecycleV1 `json:"job_lifecycle_cut"`
	ProducerScopeID string                               `json:"producer_scope_id"`
	Topology        *topologyDiagnosticArchiveSweepV1    `json:"topology_sweep_cut,omitempty"`
	LastAborted     *topologyDiagnosticArchiveAbortV1    `json:"last_aborted_sweep,omitempty"`
}

type topologyDiagnosticArchiveLifecycleV1 struct {
	State  string                                  `json:"capture_state"`
	Reason string                                  `json:"capture_reason"`
	Cut    topologyDiagnosticArchiveLifecycleCutV1 `json:"cut"`
}

type topologyDiagnosticArchiveLifecycleCutV1 struct {
	Sequence   uint64                                      `json:"sequence"`
	CapturedAt time.Time                                   `json:"captured_at"`
	Entries    []topologyDiagnosticArchiveLifecycleEntryV1 `json:"entries,omitempty"`
}

type topologyDiagnosticArchiveLifecycleEntryV1 struct {
	RegistrationID uint64                                     `json:"registration_id"`
	Hostname       string                                     `json:"hostname"`
	Port           int                                        `json:"port"`
	SNMPVersion    string                                     `json:"snmp_version"`
	LastCompleted  topologyDiagnosticArchiveLifecycleStatusV1 `json:"last_completed"`
	TopologyReady  bool                                       `json:"topology_ready"`
}

type topologyDiagnosticArchiveLifecycleStatusV1 struct {
	Phase       string    `json:"phase"`
	Outcome     string    `json:"outcome"`
	CompletedAt time.Time `json:"completed_at"`
}

type topologyDiagnosticArchiveSweepV1 struct {
	Sequence      uint64                               `json:"sequence"`
	StartedAt     time.Time                            `json:"started_at"`
	PublishedAt   time.Time                            `json:"published_at"`
	CaptureState  string                               `json:"capture_state"`
	CaptureReason string                               `json:"capture_reason"`
	RecordCount   uint64                               `json:"record_count"`
	LogicalBytes  uint64                               `json:"logical_bytes"`
	Devices       []topologyDiagnosticArchiveDeviceV1  `json:"devices,omitempty"`
	Removed       []topologyDiagnosticArchiveRemovedV1 `json:"removed_devices,omitempty"`
}

type topologyDiagnosticArchiveDeviceV1 struct {
	RegistrationID  uint64                                  `json:"registration_id"`
	Selected        bool                                    `json:"selected"`
	Outcome         string                                  `json:"outcome"`
	LastAttempt     time.Time                               `json:"last_attempt"`
	LastSuccess     time.Time                               `json:"last_success"`
	NextRetry       time.Time                               `json:"next_retry"`
	RetainedSuccess *topologyDiagnosticArchiveEvidenceRefV1 `json:"retained_success,omitempty"`
	Captures        []topologyDiagnosticArchiveCaptureV1    `json:"captures,omitempty"`
	HasObservation  bool                                    `json:"has_observation"`
	ExpiresAt       time.Time                               `json:"expires_at"`
	Renderable      bool                                    `json:"renderable"`
	Expired         bool                                    `json:"expired"`
}

type topologyDiagnosticArchiveRemovedV1 struct {
	RegistrationID  uint64                                  `json:"registration_id"`
	RetainedSuccess *topologyDiagnosticArchiveEvidenceRefV1 `json:"retained_success,omitempty"`
}

type topologyDiagnosticArchiveEvidenceRefV1 struct {
	RegistrationID uint64 `json:"registration_id"`
	Generation     uint64 `json:"generation"`
}

type topologyDiagnosticArchiveCaptureV1 struct {
	Roles          []string                                        `json:"roles"`
	AttemptOrdinal uint64                                          `json:"attempt_ordinal"`
	State          string                                          `json:"capture_state"`
	Reason         string                                          `json:"capture_reason"`
	RecordCount    uint64                                          `json:"record_count"`
	LogicalBytes   uint64                                          `json:"logical_bytes"`
	Evidence       *topologyDiagnosticArchiveAcquisitionEvidenceV1 `json:"evidence,omitempty"`
}

type topologyDiagnosticArchiveAbortV1 struct {
	Sequence              uint64    `json:"sequence"`
	StartedAt             time.Time `json:"started_at"`
	AbortedAt             time.Time `json:"aborted_at"`
	Reason                string    `json:"reason"`
	Phase                 string    `json:"phase"`
	ActiveRegistrationID  uint64    `json:"active_registration_id,omitempty"`
	HasActiveRegistration bool      `json:"has_active_registration"`
	RegistrationCount     int       `json:"registration_count"`
	SelectedCount         int       `json:"selected_count"`
}

const (
	topologyDiagnosticArchiveCaptureRoleLatestAttempt   = "latest_attempt"
	topologyDiagnosticArchiveCaptureRoleRetainedSuccess = "retained_success"
)

func writeTopologyDiagnosticArchive(w io.Writer, diagnostics topologyDiagnostics) error {
	return writeTopologyDiagnosticArchiveWithProducerVersion(w, diagnostics, buildinfo.Version)
}

func writeTopologyDiagnosticArchiveWithProducerVersion(
	w io.Writer,
	diagnostics topologyDiagnostics,
	producerVersion string,
) error {
	if w == nil {
		return errors.New("write SNMP topology diagnostic archive: nil writer")
	}
	document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, producerVersion)
	if err != nil {
		return fmt.Errorf("write SNMP topology diagnostic archive: %w", err)
	}
	encoder, err := zstd.NewWriter(
		w,
		zstd.WithEncoderLevel(zstd.SpeedDefault),
		zstd.WithEncoderConcurrency(1),
		zstd.WithEncoderCRC(true),
	)
	if err != nil {
		return fmt.Errorf("write SNMP topology diagnostic archive: create zstd encoder: %w", err)
	}
	encodeErr := jsonv2.MarshalWrite(encoder, document, topologyDiagnosticArchiveWriterJSONOptions)
	closeErr := encoder.Close()
	if encodeErr != nil {
		return fmt.Errorf("write SNMP topology diagnostic archive: encode JSON: %w", encodeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("write SNMP topology diagnostic archive: close zstd encoder: %w", closeErr)
	}
	return nil
}

func readTopologyDiagnosticArchive(
	r io.Reader,
	limits topologyDiagnosticArchiveReadLimits,
) (topologyDiagnosticArchive, error) {
	if r == nil {
		return topologyDiagnosticArchive{}, errors.New("read SNMP topology diagnostic archive: nil reader")
	}
	if limits.maxCompressedBytes <= 0 || limits.maxCompressedBytes == math.MaxInt64 {
		return topologyDiagnosticArchive{}, errors.New("read SNMP topology diagnostic archive: invalid compressed-byte limit")
	}
	if limits.maxDecodedBytes <= 0 || limits.maxDecodedBytes == math.MaxInt64 {
		return topologyDiagnosticArchive{}, errors.New("read SNMP topology diagnostic archive: invalid decoded-byte limit")
	}

	compressed := &io.LimitedReader{R: r, N: limits.maxCompressedBytes + 1}
	decoder, err := zstd.NewReader(compressed, zstd.WithDecoderConcurrency(1))
	if err != nil {
		if compressed.N == 0 {
			return topologyDiagnosticArchive{}, errTopologyDiagnosticArchiveCompressedLimit
		}
		return topologyDiagnosticArchive{}, fmt.Errorf("read SNMP topology diagnostic archive: create zstd decoder: %w", err)
	}

	decoded := &io.LimitedReader{R: decoder, N: limits.maxDecodedBytes + 1}
	var document topologyDiagnosticArchiveDocumentV1
	err = jsonv2.UnmarshalRead(decoded, &document, topologyDiagnosticArchiveReaderJSONOptions)
	decoder.Close()
	if compressed.N == 0 {
		return topologyDiagnosticArchive{}, errTopologyDiagnosticArchiveCompressedLimit
	}
	if decoded.N == 0 {
		return topologyDiagnosticArchive{}, errTopologyDiagnosticArchiveDecodedLimit
	}
	if err != nil {
		return topologyDiagnosticArchive{}, fmt.Errorf("read SNMP topology diagnostic archive: decode JSON: %w", err)
	}
	diagnostics, err := document.diagnostics()
	if err != nil {
		return topologyDiagnosticArchive{}, fmt.Errorf("read SNMP topology diagnostic archive: %w", err)
	}
	return topologyDiagnosticArchive{
		producerVersion: document.Producer.AgentVersion,
		diagnostics:     diagnostics,
	}, nil
}

func newTopologyDiagnosticArchiveDocumentV1(
	diagnostics topologyDiagnostics,
	producerVersion string,
) (topologyDiagnosticArchiveDocumentV1, error) {
	snapshot, err := newTopologyDiagnosticArchiveSnapshotV1(diagnostics)
	if err != nil {
		return topologyDiagnosticArchiveDocumentV1{}, err
	}
	return topologyDiagnosticArchiveDocumentV1{
		Format:  topologyDiagnosticArchiveFormat,
		Version: topologyDiagnosticArchiveVersion,
		Producer: topologyDiagnosticArchiveProducerV1{
			AgentVersion: producerVersion,
		},
		Snapshot: snapshot,
	}, nil
}

func newTopologyDiagnosticArchiveSnapshotV1(
	diagnostics topologyDiagnostics,
) (topologyDiagnosticArchiveSnapshotV1, error) {
	lifecycle, err := newTopologyDiagnosticArchiveLifecycleV1(diagnostics.lifecycle)
	if err != nil {
		return topologyDiagnosticArchiveSnapshotV1{}, err
	}
	result := topologyDiagnosticArchiveSnapshotV1{
		Lifecycle:       lifecycle,
		ProducerScopeID: diagnostics.producerScopeID,
	}
	if diagnostics.topology != nil {
		cut, err := newTopologyDiagnosticArchiveSweepV1(diagnostics.topology)
		if err != nil {
			return topologyDiagnosticArchiveSnapshotV1{}, err
		}
		result.Topology = &cut
	}
	if diagnostics.lastAborted != nil {
		abort, err := newTopologyDiagnosticArchiveAbortV1(diagnostics.lastAborted)
		if err != nil {
			return topologyDiagnosticArchiveSnapshotV1{}, err
		}
		result.LastAborted = &abort
	}
	return result, nil
}

func newTopologyDiagnosticArchiveLifecycleV1(
	lifecycle topologyJobLifecycleDiagnosticCut,
) (topologyDiagnosticArchiveLifecycleV1, error) {
	state, err := topologyDiagnosticArchiveCaptureStateName(lifecycle.state)
	if err != nil {
		return topologyDiagnosticArchiveLifecycleV1{}, fmt.Errorf("job lifecycle capture state: %w", err)
	}
	reason, err := topologyDiagnosticArchiveCaptureReasonName(lifecycle.reason)
	if err != nil {
		return topologyDiagnosticArchiveLifecycleV1{}, fmt.Errorf("job lifecycle capture reason: %w", err)
	}
	result := topologyDiagnosticArchiveLifecycleV1{
		State:  state,
		Reason: reason,
		Cut: topologyDiagnosticArchiveLifecycleCutV1{
			Sequence:   lifecycle.cut.Sequence,
			CapturedAt: lifecycle.cut.CapturedAt,
			Entries:    make([]topologyDiagnosticArchiveLifecycleEntryV1, 0, len(lifecycle.cut.Entries)),
		},
	}
	for _, entry := range lifecycle.cut.Entries {
		phase, err := topologyDiagnosticArchiveLifecyclePhaseName(entry.LastCompleted.Phase)
		if err != nil {
			return topologyDiagnosticArchiveLifecycleV1{}, fmt.Errorf("job lifecycle registration %d phase: %w", entry.RegistrationID, err)
		}
		outcome, err := topologyDiagnosticArchiveLifecycleOutcomeName(entry.LastCompleted.Outcome)
		if err != nil {
			return topologyDiagnosticArchiveLifecycleV1{}, fmt.Errorf("job lifecycle registration %d outcome: %w", entry.RegistrationID, err)
		}
		result.Cut.Entries = append(result.Cut.Entries, topologyDiagnosticArchiveLifecycleEntryV1{
			RegistrationID: uint64(entry.RegistrationID),
			Hostname:       entry.Info.Hostname,
			Port:           entry.Info.Port,
			SNMPVersion:    entry.Info.SNMPVersion,
			LastCompleted: topologyDiagnosticArchiveLifecycleStatusV1{
				Phase:       phase,
				Outcome:     outcome,
				CompletedAt: entry.LastCompleted.CompletedAt,
			},
			TopologyReady: entry.TopologyReady,
		})
	}
	return result, nil
}

func newTopologyDiagnosticArchiveSweepV1(cut *topologySweepDiagnosticCut) (topologyDiagnosticArchiveSweepV1, error) {
	state, err := topologyDiagnosticArchiveCaptureStateName(cut.captureState)
	if err != nil {
		return topologyDiagnosticArchiveSweepV1{}, fmt.Errorf("topology sweep capture state: %w", err)
	}
	reason, err := topologyDiagnosticArchiveCaptureReasonName(cut.captureReason)
	if err != nil {
		return topologyDiagnosticArchiveSweepV1{}, fmt.Errorf("topology sweep capture reason: %w", err)
	}
	result := topologyDiagnosticArchiveSweepV1{
		Sequence:      cut.sequence,
		StartedAt:     cut.startedAt,
		PublishedAt:   cut.publishedAt,
		CaptureState:  state,
		CaptureReason: reason,
		RecordCount:   cut.recordCount,
		LogicalBytes:  cut.logicalBytes,
		Devices:       make([]topologyDiagnosticArchiveDeviceV1, 0, len(cut.devices)),
		Removed:       make([]topologyDiagnosticArchiveRemovedV1, 0, len(cut.removed)),
	}
	for _, device := range cut.devices {
		row, err := newTopologyDiagnosticArchiveDeviceV1(device)
		if err != nil {
			return topologyDiagnosticArchiveSweepV1{}, err
		}
		result.Devices = append(result.Devices, row)
	}
	for _, removed := range cut.removed {
		result.Removed = append(result.Removed, topologyDiagnosticArchiveRemovedV1{
			RegistrationID:  uint64(removed.registrationID),
			RetainedSuccess: topologyDiagnosticArchiveEvidenceRef(removed.retainedSuccess, removed.hasRetainedSuccess),
		})
	}
	return result, nil
}

func newTopologyDiagnosticArchiveDeviceV1(
	device topologySweepDeviceDiagnostic,
) (topologyDiagnosticArchiveDeviceV1, error) {
	outcome, err := topologyDiagnosticArchiveDeviceOutcomeName(device.outcome)
	if err != nil {
		return topologyDiagnosticArchiveDeviceV1{}, fmt.Errorf("topology sweep registration %d outcome: %w", device.registrationID, err)
	}
	result := topologyDiagnosticArchiveDeviceV1{
		RegistrationID:  uint64(device.registrationID),
		Selected:        device.selected,
		Outcome:         outcome,
		LastAttempt:     device.lastAttempt,
		LastSuccess:     device.lastSuccess,
		NextRetry:       device.nextRetry,
		RetainedSuccess: topologyDiagnosticArchiveEvidenceRef(device.retainedSuccess, device.hasRetainedSuccess),
		HasObservation:  device.hasObservation,
		ExpiresAt:       device.expiresAt,
		Renderable:      device.renderable,
		Expired:         device.expired,
	}
	if device.acquisition != nil {
		capture, err := newTopologyDiagnosticArchiveCaptureV1(
			device.registrationID,
			device.acquisition,
			[]string{topologyDiagnosticArchiveCaptureRoleRetainedSuccess},
		)
		if err != nil {
			return topologyDiagnosticArchiveDeviceV1{}, err
		}
		result.Captures = append(result.Captures, capture)
	}
	if device.latestAttempt != nil {
		if device.latestAttempt == device.acquisition {
			result.Captures[0].Roles = append(
				[]string{topologyDiagnosticArchiveCaptureRoleLatestAttempt},
				result.Captures[0].Roles...,
			)
		} else {
			capture, err := newTopologyDiagnosticArchiveCaptureV1(
				device.registrationID,
				device.latestAttempt,
				[]string{topologyDiagnosticArchiveCaptureRoleLatestAttempt},
			)
			if err != nil {
				return topologyDiagnosticArchiveDeviceV1{}, err
			}
			result.Captures = append(result.Captures, capture)
		}
	}
	return result, nil
}

func newTopologyDiagnosticArchiveCaptureV1(
	registrationID ddsnmp.DeviceRegistrationID,
	capture *topologyAcquisitionCapture,
	roles []string,
) (topologyDiagnosticArchiveCaptureV1, error) {
	state, err := topologyDiagnosticArchiveCaptureStateName(capture.state)
	if err != nil {
		return topologyDiagnosticArchiveCaptureV1{}, fmt.Errorf("topology sweep registration %d capture state: %w", registrationID, err)
	}
	reason, err := topologyDiagnosticArchiveCaptureReasonName(capture.reason)
	if err != nil {
		return topologyDiagnosticArchiveCaptureV1{}, fmt.Errorf("topology sweep registration %d capture reason: %w", registrationID, err)
	}
	if capture.attemptID.registrationID != registrationID {
		return topologyDiagnosticArchiveCaptureV1{}, fmt.Errorf(
			"topology sweep registration %d capture belongs to registration %d",
			registrationID,
			capture.attemptID.registrationID,
		)
	}
	if capture.attemptID.ordinal == 0 {
		return topologyDiagnosticArchiveCaptureV1{}, fmt.Errorf("topology sweep registration %d capture attempt ordinal is zero", registrationID)
	}
	if capture.state == diagnosticCaptureAvailable && capture.evidence == nil {
		return topologyDiagnosticArchiveCaptureV1{}, fmt.Errorf("topology sweep registration %d available capture has no evidence", registrationID)
	}
	if capture.state != diagnosticCaptureAvailable && capture.evidence != nil {
		return topologyDiagnosticArchiveCaptureV1{}, fmt.Errorf("topology sweep registration %d unavailable capture has evidence", registrationID)
	}
	result := topologyDiagnosticArchiveCaptureV1{
		Roles:          roles,
		AttemptOrdinal: capture.attemptID.ordinal,
		State:          state,
		Reason:         reason,
		RecordCount:    capture.recordCount,
		LogicalBytes:   capture.logicalBytes,
	}
	if capture.evidence != nil {
		if capture.evidence.id != capture.attemptID {
			return topologyDiagnosticArchiveCaptureV1{}, fmt.Errorf("topology sweep registration %d capture/evidence attempt mismatch", registrationID)
		}
		evidence, err := newTopologyDiagnosticArchiveAcquisitionEvidenceV1(capture.evidence)
		if err != nil {
			return topologyDiagnosticArchiveCaptureV1{}, fmt.Errorf("topology sweep registration %d capture evidence: %w", registrationID, err)
		}
		result.Evidence = &evidence
	}
	return result, nil
}

func topologyDiagnosticArchiveEvidenceRef(
	ref topologyEvidenceRef,
	present bool,
) *topologyDiagnosticArchiveEvidenceRefV1 {
	if !present {
		return nil
	}
	return &topologyDiagnosticArchiveEvidenceRefV1{
		RegistrationID: uint64(ref.registrationID),
		Generation:     ref.generation,
	}
}

func newTopologyDiagnosticArchiveAbortV1(
	abort *topologyAbortedSweepDiagnostic,
) (topologyDiagnosticArchiveAbortV1, error) {
	reason, err := topologyDiagnosticArchiveAbortReasonName(abort.reason)
	if err != nil {
		return topologyDiagnosticArchiveAbortV1{}, err
	}
	phase, err := topologyDiagnosticArchiveSweepPhaseName(abort.phase)
	if err != nil {
		return topologyDiagnosticArchiveAbortV1{}, err
	}
	return topologyDiagnosticArchiveAbortV1{
		Sequence:              abort.sequence,
		StartedAt:             abort.startedAt,
		AbortedAt:             abort.abortedAt,
		Reason:                reason,
		Phase:                 phase,
		ActiveRegistrationID:  uint64(abort.activeRegistrationID),
		HasActiveRegistration: abort.hasActiveRegistration,
		RegistrationCount:     abort.registrationCount,
		SelectedCount:         abort.selectedCount,
	}, nil
}

func (d topologyDiagnosticArchiveDocumentV1) diagnostics() (topologyDiagnostics, error) {
	if d.Format != topologyDiagnosticArchiveFormat {
		return topologyDiagnostics{}, fmt.Errorf("unsupported format %q", d.Format)
	}
	if d.Version != topologyDiagnosticArchiveVersion {
		return topologyDiagnostics{}, fmt.Errorf("unsupported version %d", d.Version)
	}
	return d.Snapshot.diagnostics()
}

func (s topologyDiagnosticArchiveSnapshotV1) diagnostics() (topologyDiagnostics, error) {
	lifecycle, err := s.Lifecycle.lifecycle()
	if err != nil {
		return topologyDiagnostics{}, err
	}
	result := topologyDiagnostics{
		lifecycle:       lifecycle,
		producerScopeID: s.ProducerScopeID,
	}
	if s.Topology != nil {
		cut, err := s.Topology.sweep()
		if err != nil {
			return topologyDiagnostics{}, err
		}
		result.topology = cut
	}
	if s.LastAborted != nil {
		abort, err := s.LastAborted.abort()
		if err != nil {
			return topologyDiagnostics{}, err
		}
		result.lastAborted = abort
	}
	return result, nil
}

func (l topologyDiagnosticArchiveLifecycleV1) lifecycle() (topologyJobLifecycleDiagnosticCut, error) {
	state, err := topologyDiagnosticArchiveParseCaptureState(l.State)
	if err != nil {
		return topologyJobLifecycleDiagnosticCut{}, fmt.Errorf("job lifecycle capture state: %w", err)
	}
	reason, err := topologyDiagnosticArchiveParseCaptureReason(l.Reason)
	if err != nil {
		return topologyJobLifecycleDiagnosticCut{}, fmt.Errorf("job lifecycle capture reason: %w", err)
	}
	result := topologyJobLifecycleDiagnosticCut{
		state:  state,
		reason: reason,
		cut: ddsnmp.DeviceLifecycleCut{
			Sequence:   l.Cut.Sequence,
			CapturedAt: l.Cut.CapturedAt,
			Entries:    make([]ddsnmp.DeviceLifecycleEntry, 0, len(l.Cut.Entries)),
		},
	}
	seen := make(map[ddsnmp.DeviceRegistrationID]struct{}, len(l.Cut.Entries))
	for _, entry := range l.Cut.Entries {
		registrationID := ddsnmp.DeviceRegistrationID(entry.RegistrationID)
		if registrationID == 0 {
			return topologyJobLifecycleDiagnosticCut{}, errors.New("job lifecycle registration ID is zero")
		}
		if _, ok := seen[registrationID]; ok {
			return topologyJobLifecycleDiagnosticCut{}, fmt.Errorf("duplicate job lifecycle registration ID %d", registrationID)
		}
		seen[registrationID] = struct{}{}
		phase, err := topologyDiagnosticArchiveParseLifecyclePhase(entry.LastCompleted.Phase)
		if err != nil {
			return topologyJobLifecycleDiagnosticCut{}, fmt.Errorf("job lifecycle registration %d phase: %w", registrationID, err)
		}
		outcome, err := topologyDiagnosticArchiveParseLifecycleOutcome(entry.LastCompleted.Outcome)
		if err != nil {
			return topologyJobLifecycleDiagnosticCut{}, fmt.Errorf("job lifecycle registration %d outcome: %w", registrationID, err)
		}
		result.cut.Entries = append(result.cut.Entries, ddsnmp.DeviceLifecycleEntry{
			RegistrationID: registrationID,
			Info: ddsnmp.DeviceLifecycleInfo{
				Hostname:    entry.Hostname,
				Port:        entry.Port,
				SNMPVersion: entry.SNMPVersion,
			},
			LastCompleted: ddsnmp.DeviceLifecycleStatus{
				Phase:       phase,
				Outcome:     outcome,
				CompletedAt: entry.LastCompleted.CompletedAt,
			},
			TopologyReady: entry.TopologyReady,
		})
	}
	return result, nil
}

func (s topologyDiagnosticArchiveSweepV1) sweep() (*topologySweepDiagnosticCut, error) {
	state, err := topologyDiagnosticArchiveParseCaptureState(s.CaptureState)
	if err != nil {
		return nil, fmt.Errorf("topology sweep capture state: %w", err)
	}
	reason, err := topologyDiagnosticArchiveParseCaptureReason(s.CaptureReason)
	if err != nil {
		return nil, fmt.Errorf("topology sweep capture reason: %w", err)
	}
	result := &topologySweepDiagnosticCut{
		sequence:      s.Sequence,
		startedAt:     s.StartedAt,
		publishedAt:   s.PublishedAt,
		captureState:  state,
		captureReason: reason,
		recordCount:   s.RecordCount,
		logicalBytes:  s.LogicalBytes,
		devices:       make([]topologySweepDeviceDiagnostic, 0, len(s.Devices)),
		removed:       make([]topologyRemovedDeviceDiagnostic, 0, len(s.Removed)),
	}
	seen := make(map[ddsnmp.DeviceRegistrationID]struct{}, len(s.Devices)+len(s.Removed))
	for _, device := range s.Devices {
		row, err := device.device(s.Sequence)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[row.registrationID]; ok {
			return nil, fmt.Errorf("duplicate topology sweep registration ID %d", row.registrationID)
		}
		seen[row.registrationID] = struct{}{}
		result.devices = append(result.devices, row)
	}
	for _, removed := range s.Removed {
		registrationID := ddsnmp.DeviceRegistrationID(removed.RegistrationID)
		if registrationID == 0 {
			return nil, errors.New("removed topology registration ID is zero")
		}
		if _, ok := seen[registrationID]; ok {
			return nil, fmt.Errorf("duplicate topology sweep registration ID %d", registrationID)
		}
		seen[registrationID] = struct{}{}
		ref, hasRef, err := removed.RetainedSuccess.evidenceRef(registrationID)
		if err != nil {
			return nil, fmt.Errorf("removed topology registration %d: %w", registrationID, err)
		}
		if hasRef && ref.generation > s.Sequence {
			return nil, fmt.Errorf(
				"removed topology registration %d retained-success generation %d exceeds sweep generation %d",
				registrationID,
				ref.generation,
				s.Sequence,
			)
		}
		result.removed = append(result.removed, topologyRemovedDeviceDiagnostic{
			registrationID:     registrationID,
			retainedSuccess:    ref,
			hasRetainedSuccess: hasRef,
		})
	}
	return result, nil
}

func (d topologyDiagnosticArchiveDeviceV1) device(sweepGeneration uint64) (topologySweepDeviceDiagnostic, error) {
	registrationID := ddsnmp.DeviceRegistrationID(d.RegistrationID)
	if registrationID == 0 {
		return topologySweepDeviceDiagnostic{}, errors.New("topology sweep registration ID is zero")
	}
	outcome, err := topologyDiagnosticArchiveParseDeviceOutcome(d.Outcome)
	if err != nil {
		return topologySweepDeviceDiagnostic{}, fmt.Errorf("topology sweep registration %d outcome: %w", registrationID, err)
	}
	ref, hasRef, err := d.RetainedSuccess.evidenceRef(registrationID)
	if err != nil {
		return topologySweepDeviceDiagnostic{}, fmt.Errorf("topology sweep registration %d: %w", registrationID, err)
	}
	result := topologySweepDeviceDiagnostic{
		registrationID:     registrationID,
		selected:           d.Selected,
		outcome:            outcome,
		lastAttempt:        d.LastAttempt,
		lastSuccess:        d.LastSuccess,
		nextRetry:          d.NextRetry,
		retainedSuccess:    ref,
		hasRetainedSuccess: hasRef,
		hasObservation:     d.HasObservation,
		expiresAt:          d.ExpiresAt,
		renderable:         d.Renderable,
		expired:            d.Expired,
	}
	seenRoles := make(map[string]struct{}, 2)
	seenAttemptOrdinals := make(map[uint64]struct{}, len(d.Captures))
	for _, archivedCapture := range d.Captures {
		capture, err := archivedCapture.capture(registrationID)
		if err != nil {
			return topologySweepDeviceDiagnostic{}, err
		}
		if _, ok := seenAttemptOrdinals[capture.attemptID.ordinal]; ok {
			return topologySweepDeviceDiagnostic{}, fmt.Errorf(
				"topology sweep registration %d duplicate capture attempt ordinal %d",
				registrationID,
				capture.attemptID.ordinal,
			)
		}
		seenAttemptOrdinals[capture.attemptID.ordinal] = struct{}{}
		if len(archivedCapture.Roles) == 0 {
			return topologySweepDeviceDiagnostic{}, fmt.Errorf("topology sweep registration %d capture has no role", registrationID)
		}
		for _, role := range archivedCapture.Roles {
			if _, ok := seenRoles[role]; ok {
				return topologySweepDeviceDiagnostic{}, fmt.Errorf("topology sweep registration %d duplicate capture role %q", registrationID, role)
			}
			seenRoles[role] = struct{}{}
			switch role {
			case topologyDiagnosticArchiveCaptureRoleLatestAttempt:
				result.latestAttempt = capture
			case topologyDiagnosticArchiveCaptureRoleRetainedSuccess:
				result.acquisition = capture
			default:
				return topologySweepDeviceDiagnostic{}, fmt.Errorf("topology sweep registration %d unknown capture role %q", registrationID, role)
			}
		}
	}
	if hasRef != (result.acquisition != nil) {
		return topologySweepDeviceDiagnostic{}, fmt.Errorf(
			"topology sweep registration %d retained-success reference and capture role disagree",
			registrationID,
		)
	}
	if hasRef && ref.generation > sweepGeneration {
		return topologySweepDeviceDiagnostic{}, fmt.Errorf(
			"topology sweep registration %d retained-success generation %d exceeds sweep generation %d",
			registrationID,
			ref.generation,
			sweepGeneration,
		)
	}
	if result.acquisition != nil && result.acquisition.state == diagnosticCaptureAvailable {
		if err := validateTopologyAcquisitionEvidence(result.acquisition.evidence); err != nil {
			return topologySweepDeviceDiagnostic{}, fmt.Errorf(
				"topology sweep registration %d retained-success evidence: %w",
				registrationID,
				err,
			)
		}
	}
	return result, nil
}

func (c topologyDiagnosticArchiveCaptureV1) capture(
	registrationID ddsnmp.DeviceRegistrationID,
) (*topologyAcquisitionCapture, error) {
	state, err := topologyDiagnosticArchiveParseCaptureState(c.State)
	if err != nil {
		return nil, fmt.Errorf("topology sweep registration %d capture state: %w", registrationID, err)
	}
	reason, err := topologyDiagnosticArchiveParseCaptureReason(c.Reason)
	if err != nil {
		return nil, fmt.Errorf("topology sweep registration %d capture reason: %w", registrationID, err)
	}
	attemptID := topologyAcquisitionAttemptID{registrationID: registrationID, ordinal: c.AttemptOrdinal}
	if attemptID.ordinal == 0 {
		return nil, fmt.Errorf("topology sweep registration %d capture attempt ordinal is zero", registrationID)
	}
	result := &topologyAcquisitionCapture{
		attemptID:    attemptID,
		state:        state,
		reason:       reason,
		recordCount:  c.RecordCount,
		logicalBytes: c.LogicalBytes,
	}
	if c.Evidence != nil {
		evidence, err := c.Evidence.evidence(attemptID)
		if err != nil {
			return nil, fmt.Errorf("topology sweep registration %d capture evidence: %w", registrationID, err)
		}
		result.evidence = evidence
	}
	if state == diagnosticCaptureAvailable && result.evidence == nil {
		return nil, fmt.Errorf("topology sweep registration %d available capture has no evidence", registrationID)
	}
	if state != diagnosticCaptureAvailable && result.evidence != nil {
		return nil, fmt.Errorf("topology sweep registration %d unavailable capture has evidence", registrationID)
	}
	return result, nil
}

func (r *topologyDiagnosticArchiveEvidenceRefV1) evidenceRef(
	owner ddsnmp.DeviceRegistrationID,
) (topologyEvidenceRef, bool, error) {
	if r == nil {
		return topologyEvidenceRef{}, false, nil
	}
	registrationID := ddsnmp.DeviceRegistrationID(r.RegistrationID)
	if registrationID == 0 || registrationID != owner {
		return topologyEvidenceRef{}, false, fmt.Errorf("retained-success registration reference %d does not match owner %d", registrationID, owner)
	}
	if r.Generation == 0 {
		return topologyEvidenceRef{}, false, errors.New("retained-success generation is zero")
	}
	return topologyEvidenceRef{registrationID: registrationID, generation: r.Generation}, true, nil
}

func (a topologyDiagnosticArchiveAbortV1) abort() (*topologyAbortedSweepDiagnostic, error) {
	reason, err := topologyDiagnosticArchiveParseAbortReason(a.Reason)
	if err != nil {
		return nil, err
	}
	phase, err := topologyDiagnosticArchiveParseSweepPhase(a.Phase)
	if err != nil {
		return nil, err
	}
	registrationID := ddsnmp.DeviceRegistrationID(a.ActiveRegistrationID)
	if a.HasActiveRegistration && registrationID == 0 {
		return nil, errors.New("aborted sweep active registration ID is zero")
	}
	if !a.HasActiveRegistration && registrationID != 0 {
		return nil, errors.New("aborted sweep has an inactive registration reference")
	}
	if a.RegistrationCount < 0 || a.SelectedCount < 0 || a.SelectedCount > a.RegistrationCount {
		return nil, errors.New("aborted sweep registration counts are invalid")
	}
	return &topologyAbortedSweepDiagnostic{
		sequence:              a.Sequence,
		startedAt:             a.StartedAt,
		abortedAt:             a.AbortedAt,
		reason:                reason,
		phase:                 phase,
		activeRegistrationID:  registrationID,
		hasActiveRegistration: a.HasActiveRegistration,
		registrationCount:     a.RegistrationCount,
		selectedCount:         a.SelectedCount,
	}, nil
}

var (
	topologyDiagnosticArchiveCaptureStateNames = []string{
		"unknown", "available", "limit_exceeded", "unavailable",
	}
	topologyDiagnosticArchiveCaptureReasonNames = []string{
		"none", "record_limit", "byte_limit", "projection_error", "projection_panic",
		"global_record_limit", "global_byte_limit",
	}
	topologyDiagnosticArchiveLifecyclePhaseNames = []string{
		"unknown", "init", "check", "collect",
	}
	topologyDiagnosticArchiveLifecycleOutcomeNames = []string{
		"unknown", "success", "failed",
	}
	topologyDiagnosticArchiveDeviceOutcomeNames = []string{
		"unknown", "success", "no_profiles", "failed",
	}
	topologyDiagnosticArchiveAbortReasonNames = []string{
		"unknown", "canceled", "panic",
	}
	topologyDiagnosticArchiveSweepPhaseNames = []string{
		"unknown", "registration_cut", "target_resolution", "device_refresh", "commit",
	}
)

func topologyDiagnosticArchiveEnumName[T ~uint8](value T, names []string) (string, error) {
	index := int(value)
	if index < 0 || index >= len(names) {
		return "", fmt.Errorf("unknown value %d", value)
	}
	return names[index], nil
}

func topologyDiagnosticArchiveParseEnum[T ~uint8](value string, names []string) (T, error) {
	for index, name := range names {
		if value == name {
			return T(index), nil
		}
	}
	return 0, fmt.Errorf("unknown value %q", value)
}

func topologyDiagnosticArchiveCaptureStateName(value diagnosticCaptureState) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveCaptureStateNames)
}

func topologyDiagnosticArchiveParseCaptureState(value string) (diagnosticCaptureState, error) {
	return topologyDiagnosticArchiveParseEnum[diagnosticCaptureState](value, topologyDiagnosticArchiveCaptureStateNames)
}

func topologyDiagnosticArchiveCaptureReasonName(value diagnosticCaptureReason) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveCaptureReasonNames)
}

func topologyDiagnosticArchiveParseCaptureReason(value string) (diagnosticCaptureReason, error) {
	return topologyDiagnosticArchiveParseEnum[diagnosticCaptureReason](value, topologyDiagnosticArchiveCaptureReasonNames)
}

func topologyDiagnosticArchiveLifecyclePhaseName(value ddsnmp.DeviceLifecyclePhase) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveLifecyclePhaseNames)
}

func topologyDiagnosticArchiveParseLifecyclePhase(value string) (ddsnmp.DeviceLifecyclePhase, error) {
	return topologyDiagnosticArchiveParseEnum[ddsnmp.DeviceLifecyclePhase](value, topologyDiagnosticArchiveLifecyclePhaseNames)
}

func topologyDiagnosticArchiveLifecycleOutcomeName(value ddsnmp.DeviceLifecycleOutcome) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveLifecycleOutcomeNames)
}

func topologyDiagnosticArchiveParseLifecycleOutcome(value string) (ddsnmp.DeviceLifecycleOutcome, error) {
	return topologyDiagnosticArchiveParseEnum[ddsnmp.DeviceLifecycleOutcome](value, topologyDiagnosticArchiveLifecycleOutcomeNames)
}

func topologyDiagnosticArchiveDeviceOutcomeName(value deviceRefreshOutcome) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveDeviceOutcomeNames)
}

func topologyDiagnosticArchiveParseDeviceOutcome(value string) (deviceRefreshOutcome, error) {
	return topologyDiagnosticArchiveParseEnum[deviceRefreshOutcome](value, topologyDiagnosticArchiveDeviceOutcomeNames)
}

func topologyDiagnosticArchiveAbortReasonName(value topologyDiagnosticAbortReason) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveAbortReasonNames)
}

func topologyDiagnosticArchiveParseAbortReason(value string) (topologyDiagnosticAbortReason, error) {
	return topologyDiagnosticArchiveParseEnum[topologyDiagnosticAbortReason](value, topologyDiagnosticArchiveAbortReasonNames)
}

func topologyDiagnosticArchiveSweepPhaseName(value topologyDiagnosticSweepPhase) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveSweepPhaseNames)
}

func topologyDiagnosticArchiveParseSweepPhase(value string) (topologyDiagnosticSweepPhase, error) {
	return topologyDiagnosticArchiveParseEnum[topologyDiagnosticSweepPhase](value, topologyDiagnosticArchiveSweepPhaseNames)
}
