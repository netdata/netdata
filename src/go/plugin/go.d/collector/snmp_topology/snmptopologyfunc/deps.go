// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopologyfunc

import topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"

// Deps defines the dependency surface required by SNMP topology function handlers.
type Deps interface {
	Snapshot(QueryOptions) (topologyv1.Data, bool, error)
	ManagedDeviceFocusTargets() []ManagedFocusTarget
}

// ManagedFocusTarget describes a managed SNMP device that can be used as a focus root.
type ManagedFocusTarget struct {
	Value string
	Name  string
}

// QueryOptions is the function-level query shape consumed by the collector adapter.
type QueryOptions struct {
	CollapseActorsByIP     bool
	EliminateNonIPInferred bool
	MapType                string
	InferenceStrategy      string
	ManagedDeviceFocus     string
	Depth                  int
}
