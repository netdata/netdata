// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"errors"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
)

// Config selects the process-fixed secrets capability. Nil means absent; an
// empty resolver or creator catalog still enables secret-reference semantics.
type Config struct {
	Resolver *secretresolver.AtomicResolver
	Creators *secretstore.CreatorCatalog
}

func (c *Config) Validate() error {
	if c != nil && (c.Resolver == nil || c.Creators == nil) {
		return errors.New("secrets: incomplete capability configuration")
	}
	return nil
}
