// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import "github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"

// DiscoverOptions controls which normalized L2 observation families contribute
// to the result.
type DiscoverOptions = model.DiscoverOptions

// Result is the deterministic L2 topology result derived from normalized
// observations.
type Result = model.Result

// Device is a discovered network device.
type Device = model.Device

// Interface is a discovered interface on a device.
type Interface = model.Interface

// Adjacency represents a direct device-to-device neighbor relation.
type Adjacency = model.Adjacency

// Attachment ties an endpoint to a device interface.
type Attachment = model.Attachment

// Enrichment carries non-structural observations that can assist correlation.
type Enrichment = model.Enrichment

// L2Observation contains one device's normalized layer-2 observations.
type L2Observation = model.L2Observation

// ObservedInterface describes one local interface seen on a device.
type ObservedInterface = model.ObservedInterface

// LLDPRemoteObservation captures one remote LLDP neighbor advertised by a device.
type LLDPRemoteObservation = model.LLDPRemoteObservation

// CDPRemoteObservation captures one remote CDP neighbor advertised by a device.
type CDPRemoteObservation = model.CDPRemoteObservation

// BridgePortObservation maps one bridge base port to an interface index.
type BridgePortObservation = model.BridgePortObservation

// FDBObservation captures one forwarding database entry from a bridge table.
type FDBObservation = model.FDBObservation

// STPPortObservation captures one spanning-tree port row.
type STPPortObservation = model.STPPortObservation

// ARPNDObservation captures one ARP or ND neighbor-table observation.
type ARPNDObservation = model.ARPNDObservation
