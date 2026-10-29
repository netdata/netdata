// SPDX-License-Identifier: GPL-3.0-or-later
//go:build !linux

package runtime

import "errors"

func renameDirectoryExclusive(_, _, _ string) error {
	return errors.New("managed runtime initialization requires Linux")
}
