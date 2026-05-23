// SPDX-License-Identifier: GPL-3.0-or-later

// Package secretstore defines control-plane contracts for go.d v2 secretstores.
//
// Current scope:
// - core data model types
// - raw-config and runtime contracts
// - manager and snapshot provider interfaces
//
// Runtime behavior (validation, normalization, publication, and resolution)
// is implemented by the service/runtime packages.
package secretstore
