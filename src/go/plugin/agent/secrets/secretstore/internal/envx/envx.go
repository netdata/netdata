// SPDX-License-Identifier: GPL-3.0-or-later

package envx

import (
	"os"
	"strings"
)

func Lookup(name string) (string, bool) {
	v, ok := os.LookupEnv(name)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(v), true
}
