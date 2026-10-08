// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
)

// CaptureNormal runs on the publisher. It deduplicates direct immutable source
// objects across attempts, initialization, caches and retained consumer output.
func (cut *normalCut) CaptureNormal() (*diagnostics.NormalDevice, error) {
	document := cut.document
	document.ProfileContext = cut.profileContext.Snapshot()
	sources := make(map[diagnostics.SourceRef]*ddsnmp.SourceOperation)
	var collision error
	add := func(operations []*ddsnmp.SourceOperation) []diagnostics.SourceRef {
		refs := make([]diagnostics.SourceRef, 0, len(operations))
		for _, operation := range operations {
			ref := diagnostics.SourceRef{ContextID: operation.ContextID, Operation: operation.Ordinal}
			if existing := sources[ref]; existing != nil && existing != operation {
				collision = fmt.Errorf("conflicting source identity %+v", ref)
			}
			sources[ref] = operation
			refs = append(refs, ref)
		}
		return refs
	}
	captureAttempt := func(native *normalAttempt) *diagnostics.NormalAttempt {
		if native == nil {
			return nil
		}
		attempt := native.document
		attempt.Sources = add(native.sources)
		attempt.Caches = make([]diagnostics.NormalCache, len(native.caches))
		for i, cache := range native.caches {
			attempt.Caches[i] = diagnostics.NormalCache{
				Kind: cache.Kind, Profile: cache.Profile, RootOID: cache.RootOID, ConfigID: cache.ConfigID,
				CreatedAt: cache.CreatedAt, ExpiresAt: cache.ExpiresAt, Marker: cache.Marker,
				OIDs: cache.OIDs, TableTags: cache.TableTags, Tags: cache.Tags, Metadata: cache.Metadata,
				Sources: add(cache.Sources),
			}
		}
		attempt.BGP.Sources = make(map[string][]diagnostics.SourceRef, len(native.bgpSources))
		for source, operations := range native.bgpSources {
			attempt.BGP.Sources[source] = add(operations)
		}
		attempt.Licensing.Sources = add(native.licenseSources)
		return &attempt
	}
	document.Initialization = add(cut.initialization)
	document.Latest = captureAttempt(cut.latest)
	if cut.failed != cut.latest {
		document.LastFailure = captureAttempt(cut.failed)
	}

	if collision != nil {
		return nil, collision
	}
	document.Sources = make([]*ddsnmp.SourceOperation, 0, len(sources))
	for _, operation := range sources {
		document.Sources = append(document.Sources, operation)
	}
	slices.SortFunc(document.Sources, func(a, b *ddsnmp.SourceOperation) int {
		return cmp.Or(cmp.Compare(a.ContextID, b.ContextID), cmp.Compare(a.Ordinal, b.Ordinal))
	})
	return &document, nil
}
