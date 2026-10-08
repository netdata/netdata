// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func (d *NormalDevice) Validate() error {
	if d == nil || d.RegistrationID == 0 || d.RuntimeID == 0 || d.Latest == nil || d.CapturedAt.IsZero() {
		return errors.New("normal evidence is missing its device or latest attempt identity")
	}
	if err := ddsnmp.ValidateSourceOperations(d.Sources); err != nil {
		return err
	}
	sources := make(map[SourceRef]*ddsnmp.SourceOperation, len(d.Sources))
	requests := make(map[SourceRef]map[string]bool, len(d.Sources))
	for _, source := range d.Sources {
		ref := SourceRef{ContextID: source.ContextID, Operation: source.Ordinal}
		if ref.ContextID == 0 || ref.Operation == 0 || source.StartedAt.IsZero() || sources[ref] != nil {
			return fmt.Errorf("invalid or duplicate normal source identity %+v", ref)
		}
		sources[ref] = source
		oids := make(map[string]bool, len(source.RequestedOIDs))
		for _, oid := range source.RequestedOIDs {
			oids[strings.TrimPrefix(oid, ".")] = true
		}
		requests[ref] = oids
	}
	check := func(refs []SourceRef) error {
		for _, ref := range refs {
			if sources[ref] == nil {
				return fmt.Errorf("missing normal source %+v", ref)
			}
		}
		return nil
	}
	if err := check(d.Initialization); err != nil {
		return err
	}
	if d.LastFailure != nil && (!d.LastFailure.Failed || d.LastFailure.ID >= d.Latest.ID) {
		return errors.New("invalid retained failure attempt")
	}
	for _, attempt := range []*NormalAttempt{d.Latest, d.LastFailure} {
		if attempt == nil {
			continue
		}
		if attempt.ID == 0 || attempt.StartedAt.IsZero() || attempt.CompletedAt.IsZero() || (attempt.Phase != "check" && attempt.Phase != "collect") {
			return errors.New("invalid normal attempt")
		}
		if !attempt.Failures.Valid() || !attempt.Failure.Valid() {
			return errors.New("invalid normal failure evidence")
		}

		for _, refs := range attempt.BGP.Sources {
			if err := check(refs); err != nil {
				return err
			}
		}
		if err := check(attempt.Licensing.Sources); err != nil {
			return err
		}
		if err := check(attempt.Sources); err != nil {
			return err
		}
		for _, cache := range attempt.Caches {
			if err := check(cache.Sources); err != nil {
				return err
			}
		}
		for _, profile := range attempt.Profiles {
			for _, route := range profile.Acquisition.Routes {
				if err := ddsnmp.ValidateProcessingEvents(route.Processing); err != nil {
					return err
				}
				for _, bindings := range [][]ddsnmp.SourceBinding{route.Sources, route.DiscardedSources} {
					for _, binding := range bindings {
						context := binding.ContextID
						if context == 0 {
							context = attempt.ID
						}
						ref := SourceRef{ContextID: context, Operation: binding.Operation}
						if !requests[ref][binding.OID] || (binding.Role != "primary" && binding.Role != "dependency") {
							return fmt.Errorf("invalid normal source binding %+v", ref)
						}
					}
				}
			}
		}
	}
	return nil
}
