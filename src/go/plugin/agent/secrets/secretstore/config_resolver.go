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

	// prepareConfig already deep-clones the raw config before calling here.
	// Build a top-level payload view to keep store identity/source metadata static
	// while avoiding another YAML round-trip clone for provider payload resolution.
	payload := make(Config, len(cfg))
	for k, v := range cfg {
		switch k {
		case keyName, keyKind, ikeySource, ikeySourceType:
			continue
		default:
			payload[k] = v
		}
	}

	if err := secretresolver.New().ResolveWithStoreResolver(ctx, payload, nil); err != nil {
		return nil, fmt.Errorf("resolving provider payload secrets: %w", err)
	}
	return payload, nil
}
