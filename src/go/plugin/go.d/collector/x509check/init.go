// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"context"
	"errors"
)

func (c *Collector) validateConfig() error {
	if c.Source == "" {
		return errors.New("source is not set")
	}
	return nil
}

func (c *Collector) initProvider(ctx context.Context) (provider, error) {
	return newProvider(ctx, c.Config)
}
