// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package runtime

import "errors"

func privateDirectory(string) error { return errors.New("runtime store requires Linux") }
func hostBoot() bootIdentity        { return bootIdentity{} }
