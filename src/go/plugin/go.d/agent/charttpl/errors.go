// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import (
	"errors"
	"fmt"
)

var (
	errDecode        = errors.New("charttpl: decode failed")
	errSemanticCheck = errors.New("charttpl: semantic validation failed")
)

// FieldError pins validation failure to one logical field path.
type FieldError struct {
	Path   string
	Reason string
}

func (e FieldError) Error() string {
	if e.Path == "" {
		return e.Reason
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Reason)
}
