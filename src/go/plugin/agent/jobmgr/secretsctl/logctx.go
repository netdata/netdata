// SPDX-License-Identifier: GPL-3.0-or-later

package secretsctl

import (
	"context"
	"log/slog"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
)

func secretStoreResolveContext(ctx context.Context, log *logger.Logger, cfg secretstore.Config) context.Context {
	if cfg == nil {
		return logger.ContextWithLogger(ctx, log)
	}
	return secretStoreResolveContextForKey(ctx, log, cfg.ExposedKey(), cfg.Kind(), cfg.Name())
}

func secretStoreResolveContextForKey(ctx context.Context, log *logger.Logger, key string, kind secretstore.StoreKind, name string) context.Context {
	if log != nil {
		log = log.With(
			slog.String("secretstore", key),
			slog.String("secretstore_kind", string(kind)),
			slog.String("secretstore_name", name),
		)
	}
	return logger.ContextWithLogger(ctx, log)
}
