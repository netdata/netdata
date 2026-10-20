// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd

package ndexec

func normalizeGroupKillError(_ int, err error) error { return err }
