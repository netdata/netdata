// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux && !darwin && !freebsd

package main

import "os"

// The cgroup-name helper is Linux-only. Other targets are build checks and
// must not trust cache identity metadata they cannot validate.
func ownedByCurrentUser(os.FileInfo) bool {
	return false
}

func hasSingleLink(os.FileInfo) bool {
	return false
}
