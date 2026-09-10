// SPDX-License-Identifier: GPL-3.0-or-later

// Package topologydiag owns immutable topology diagnostic evidence, archive
// conversion, and offline inspection. The collector records and publishes cuts;
// Semantics supplies stateless production topology interpretation for replay.
// Published evidence is borrowed, never mutated by archive or inspection code.
package topologydiag
