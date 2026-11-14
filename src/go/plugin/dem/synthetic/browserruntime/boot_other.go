// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package runtime

func hostBoot() bootIdentity { return bootIdentity{} }
