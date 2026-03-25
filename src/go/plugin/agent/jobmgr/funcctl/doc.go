// SPDX-License-Identifier: GPL-3.0-or-later

// Package funcctl owns function and method dispatch state for jobmgr.
// Runtime job lifecycle ownership remains in jobmgr, which feeds job start/stop
// events into the controller.
package funcctl
