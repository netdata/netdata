// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/legacyconfig"
)

func checkLegacy(ctx context.Context, paths []string) error {
	for i, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		file, err := openConfig(path)
		if err != nil {
			return fmt.Errorf("legacy configuration file %d: could not open file: %w", i+1, err)
		}
		_, err = legacyconfig.Parse(file)
		_ = file.Close()
		if err != nil {
			return fmt.Errorf("legacy configuration file %d: %w", i+1, err)
		}
	}
	return ctx.Err()
}
