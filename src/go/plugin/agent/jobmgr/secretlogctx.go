// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"log/slog"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

func collectorSecretResolveContext(ctx context.Context, log *logger.Logger, cfg confgroup.Config) context.Context {
	if cfg == nil {
		return logger.ContextWithLogger(ctx, log)
	}
	if log != nil {
		log = log.With(
			slog.String("collector", cfg.Module()),
			slog.String("job", cfg.Name()),
		)
	}
	return logger.ContextWithLogger(ctx, log)
}
