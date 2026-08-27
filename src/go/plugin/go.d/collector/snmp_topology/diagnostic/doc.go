// SPDX-License-Identifier: GPL-3.0-or-later

// Package diagnostic defines the versioned SNMP topology diagnostic graph.
//
// The package owns portable member identity, strict canonical encoding, bounded
// reading, capability closure validation, and the recorder contract. It does
// not serialize collector runtime structs or perform filesystem I/O.
package diagnostic
