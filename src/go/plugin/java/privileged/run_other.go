// SPDX-License-Identifier: GPL-3.0-or-later
//go:build !linux

package privileged

import (
	"errors"
	"io"
)

func Run(_ []string, _ io.Reader, _ io.Writer) error { return errors.New("java-helper requires Linux") }
