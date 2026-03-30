// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"context"
	"fmt"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
)

func resolveProviderPayload(ctx context.Context, cfg Config) (Config, error) {
	if cfg == nil {
		return nil, nil
	}

	payload := cloneConfig(cfg)
	if payload == nil {
		return nil, nil
	}

	// Keep store identity and source metadata static; only resolve provider payload.
	delete(payload, keyName)
	delete(payload, keyKind)
	delete(payload, ikeySource)
	delete(payload, ikeySourceType)

	if err := secretresolver.New().ResolveWithStoreResolver(ctx, payload, nil); err != nil {
		return nil, fmt.Errorf("resolving provider payload secrets: %w", err)
	}
	return payload, nil
}
