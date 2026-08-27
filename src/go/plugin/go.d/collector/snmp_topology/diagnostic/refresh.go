// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	KindRefreshSweep = "refresh_sweep"

	RefreshSectionGeneration = "generation"
	RefreshSectionSweep      = "sweep"
)

const (
	RefreshSelectionDue           = "due"
	RefreshSelectionSkippedNotDue = "skipped_not_due"

	TargetResolutionLiteral      = "literal"
	TargetResolutionResolved     = "resolved"
	TargetResolutionFailed       = "failed"
	TargetResolutionTimedOut     = "timed_out"
	TargetResolutionNotAttempted = "not_attempted"

	RefreshOutcomeSuccess            = "success"
	RefreshOutcomeNoProfiles         = "no_profiles"
	RefreshOutcomeClientFailed       = "client_failed"
	RefreshOutcomeConnectFailed      = "connect_failed"
	RefreshOutcomeCollectionFailed   = "collection_failed"
	RefreshOutcomeCanceledInFlight   = "canceled_in_flight"
	RefreshOutcomeCanceledNotStarted = "canceled_not_started"
	RefreshOutcomePanicInFlight      = "panic_in_flight"
	RefreshOutcomePanicNotStarted    = "panic_not_started"
	RefreshOutcomeSkippedNotDue      = "skipped_not_due"

	GenerationReferenceNone        = "none"
	GenerationReferenceAvailable   = "available"
	GenerationReferenceUnavailable = "capture_unavailable"

	RefreshPublicationPublished = "published"
	RefreshPublicationCanceled  = "not_published_canceled"
	RefreshPublicationPanic     = "not_published_panic"

	GenerationStateRefreshed = "refreshed"
	GenerationStateRetained  = "retained"
	GenerationStateExpired   = "expired"
	GenerationStateAbsent    = "absent"

	ObservationStateAvailable     = "available"
	ObservationStateUnavailable   = "capture_unavailable"
	ObservationStateNotApplicable = "not_applicable"
)

type GenerationReferenceV1 struct {
	State string      `json:"state"`
	Ref   *ContentRef `json:"ref,omitempty"`
}

func (r GenerationReferenceV1) Validate() error {
	switch r.State {
	case GenerationReferenceAvailable:
		if r.Ref == nil {
			return errors.New("available generation reference is missing")
		}
		if err := r.Ref.Validate(); err != nil {
			return err
		}
		if r.Ref.Type() != (MemberType{Kind: KindGeneration, Schema: SchemaV1}) {
			return fmt.Errorf("generation reference has type %s@%s", r.Ref.Kind, r.Ref.Schema)
		}
	case GenerationReferenceNone, GenerationReferenceUnavailable:
		if r.Ref != nil {
			return fmt.Errorf("generation reference state %q must not carry a ref", r.State)
		}
	default:
		return fmt.Errorf("unsupported generation reference state %q", r.State)
	}
	return nil
}

type RefreshDeviceIdentityV1 struct {
	Hostname      string `json:"hostname"`
	Port          int    `json:"port"`
	SNMPVersion   string `json:"snmp_version"`
	SysObjectID   string `json:"sys_object_id,omitempty"`
	SysName       string `json:"sys_name,omitempty"`
	Vendor        string `json:"vendor,omitempty"`
	Model         string `json:"model,omitempty"`
	VnodeGUID     string `json:"vnode_guid,omitempty"`
	VnodeHostname string `json:"vnode_hostname,omitempty"`
}

func (d RefreshDeviceIdentityV1) Validate() error {
	if strings.ContainsRune(d.Hostname, 0) {
		return errors.New("refresh device hostname contains NUL")
	}
	if d.Port < 0 || d.Port > 65535 {
		return errors.New("refresh device port is outside [0,65535]")
	}
	return nil
}

type RefreshRegistrationV1 struct {
	Registration     uint64                  `json:"registration"`
	Device           RefreshDeviceIdentityV1 `json:"device"`
	Selection        string                  `json:"selection"`
	TargetResolution string                  `json:"target_resolution"`
	Outcome          string                  `json:"outcome"`
}

func (r RefreshRegistrationV1) Validate() error {
	if r.Registration == 0 {
		return errors.New("refresh registration must be nonzero")
	}
	if err := r.Device.Validate(); err != nil {
		return err
	}
	if !oneOf(r.TargetResolution,
		TargetResolutionLiteral,
		TargetResolutionResolved,
		TargetResolutionFailed,
		TargetResolutionTimedOut,
		TargetResolutionNotAttempted,
	) {
		return fmt.Errorf("unsupported target resolution %q", r.TargetResolution)
	}
	switch r.Selection {
	case RefreshSelectionSkippedNotDue:
		if r.TargetResolution != TargetResolutionNotAttempted || r.Outcome != RefreshOutcomeSkippedNotDue {
			return errors.New("skipped refresh registration has an invalid terminal shape")
		}
	case RefreshSelectionDue:
		if !oneOf(r.Outcome,
			RefreshOutcomeSuccess,
			RefreshOutcomeNoProfiles,
			RefreshOutcomeClientFailed,
			RefreshOutcomeConnectFailed,
			RefreshOutcomeCollectionFailed,
			RefreshOutcomeCanceledInFlight,
			RefreshOutcomeCanceledNotStarted,
			RefreshOutcomePanicInFlight,
			RefreshOutcomePanicNotStarted,
		) {
			return fmt.Errorf("unsupported due refresh outcome %q", r.Outcome)
		}
	default:
		return fmt.Errorf("unsupported refresh selection %q", r.Selection)
	}
	return nil
}

type RefreshPublicationV1 struct {
	State      string                `json:"state"`
	Generation GenerationReferenceV1 `json:"generation"`
}

func (p RefreshPublicationV1) Validate() error {
	if err := p.Generation.Validate(); err != nil {
		return err
	}
	switch p.State {
	case RefreshPublicationPublished:
		if p.Generation.State != GenerationReferenceAvailable {
			return errors.New("published sweep requires its generation reference")
		}
	case RefreshPublicationCanceled, RefreshPublicationPanic:
		if p.Generation.State != GenerationReferenceNone {
			return errors.New("unpublished sweep must not carry a resulting generation")
		}
	default:
		return fmt.Errorf("unsupported refresh publication state %q", p.State)
	}
	return nil
}

type RefreshSweepV1 struct {
	CaptureID          uint64                  `json:"capture_id"`
	StartedAt          string                  `json:"started_at"`
	FinishedAt         string                  `json:"finished_at"`
	PreviousGeneration GenerationReferenceV1   `json:"previous_generation"`
	Registrations      []RefreshRegistrationV1 `json:"registrations"`
	Publication        RefreshPublicationV1    `json:"publication"`
}

func (s RefreshSweepV1) Validate() error {
	if s.CaptureID == 0 {
		return errors.New("refresh sweep capture_id must be nonzero")
	}
	if err := validateCanonicalTime(s.StartedAt); err != nil {
		return fmt.Errorf("started_at: %w", err)
	}
	if err := validateCanonicalTime(s.FinishedAt); err != nil {
		return fmt.Errorf("finished_at: %w", err)
	}
	startedAt, _ := time.Parse(time.RFC3339Nano, s.StartedAt)
	finishedAt, _ := time.Parse(time.RFC3339Nano, s.FinishedAt)
	if finishedAt.Before(startedAt) {
		return errors.New("refresh sweep finished before it started")
	}
	if err := s.PreviousGeneration.Validate(); err != nil {
		return fmt.Errorf("previous_generation: %w", err)
	}
	if err := s.Publication.Validate(); err != nil {
		return fmt.Errorf("publication: %w", err)
	}
	var previous uint64
	for i, registration := range s.Registrations {
		if err := registration.Validate(); err != nil {
			return fmt.Errorf("registrations[%d]: %w", i, err)
		}
		if registration.Registration <= previous {
			return errors.New("refresh registrations must be strictly registration ordered")
		}
		previous = registration.Registration
	}
	return nil
}

func (s RefreshSweepV1) References() []ContentRef {
	refs := make([]ContentRef, 0, 2)
	if s.PreviousGeneration.Ref != nil {
		refs = append(refs, *s.PreviousGeneration.Ref)
	}
	if s.Publication.Generation.Ref != nil {
		refs = append(refs, *s.Publication.Generation.Ref)
	}
	return refs
}

type GenerationDeviceV1 struct {
	Registration     uint64      `json:"registration"`
	State            string      `json:"state"`
	Renderable       bool        `json:"renderable"`
	ObservationState string      `json:"observation_state"`
	Observation      *ContentRef `json:"observation,omitempty"`
}

func (d GenerationDeviceV1) Validate() error {
	if d.Registration == 0 {
		return errors.New("generation device registration must be nonzero")
	}
	if !oneOf(d.State, GenerationStateRefreshed, GenerationStateRetained, GenerationStateExpired, GenerationStateAbsent) {
		return fmt.Errorf("unsupported generation device state %q", d.State)
	}
	if d.Renderable && (d.State == GenerationStateExpired || d.State == GenerationStateAbsent) {
		return errors.New("expired or absent generation device cannot be renderable")
	}
	switch d.ObservationState {
	case ObservationStateAvailable:
		if !d.Renderable || d.Observation == nil {
			return errors.New("available observation requires a renderable device and a reference")
		}
		if err := d.Observation.Validate(); err != nil {
			return err
		}
		if d.Observation.Type() != (MemberType{Kind: KindObservation, Schema: SchemaV1}) {
			return fmt.Errorf("device observation has type %s@%s", d.Observation.Kind, d.Observation.Schema)
		}
	case ObservationStateUnavailable:
		if !d.Renderable || d.Observation != nil {
			return errors.New("capture-unavailable observation requires a renderable device without a reference")
		}
	case ObservationStateNotApplicable:
		if d.Renderable || d.Observation != nil {
			return errors.New("not-applicable observation requires a non-renderable device")
		}
	default:
		return fmt.Errorf("unsupported observation state %q", d.ObservationState)
	}
	return nil
}
