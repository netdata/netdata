// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"fmt"
	"strings"
)

// SplitFunctionName splits a function name into module and method parts.
// Expected format: module:method.
func SplitFunctionName(name string) (string, string, error) {
	parts := strings.SplitN(name, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid function name '%s' (expected module:method)", name)
	}
	return parts[0], parts[1], nil
}
