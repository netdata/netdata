// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package browserruntime

func hostBoot() bootIdentity { return bootIdentity{} }
