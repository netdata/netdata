// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"errors"
	"fmt"
	"unicode"
)

// ValidateJobName checks that a job name contains no spaces, dots, or colons.
func ValidateJobName(jobName string) error {
	for _, r := range jobName {
		if unicode.IsSpace(r) {
			return errors.New("contains spaces")
		}
		switch r {
		case '.', ':':
			return fmt.Errorf("contains '%c'", r)
		}
	}
	return nil
}
