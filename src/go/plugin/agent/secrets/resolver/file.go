// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (r *Resolver) resolveFile(_ context.Context, path, original string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("resolving secret '%s': file path must be absolute, got '%s'", original, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': %w", original, err)
	}
	return strings.TrimSpace(string(data)), nil
}
