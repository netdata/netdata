// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package privilege

func capabilitiesAtExec() (bool, error) { return false, nil }
