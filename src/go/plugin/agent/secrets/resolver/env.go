// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"context"
	"fmt"
	"os"
	"strings"
)

func (r *Resolver) resolveEnv(_ context.Context, name, original string) (string, error) {
	val, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("resolving secret '%s': environment variable '%s' is not set", original, name)
	}
	return strings.TrimSpace(val), nil
}
