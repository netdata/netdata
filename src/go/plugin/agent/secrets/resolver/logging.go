// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"context"

	"github.com/netdata/netdata/go/plugins/logger"
)

func logResolved(ctx context.Context, format string, args ...any) {
	if log, ok := logger.LoggerFromContext(ctx); ok {
		log.Infof(format, args...)
	}
}
