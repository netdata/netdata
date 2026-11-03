// SPDX-License-Identifier: GPL-3.0-or-later

package supervisord

import (
	"context"
	"errors"
	"fmt"
	"net/url"
)

func (c *Collector) verifyConfig() error {
	if c.URL == "" {
		return errors.New("'url' not set")
	}
	return nil
}

func (c *Collector) initSupervisorClient(ctx context.Context) (supervisorClient, error) {
	u, err := url.Parse(c.URL)
	if err != nil {
		return nil, fmt.Errorf("parse 'url': %v (%s)", err, c.URL)
	}
	return newSupervisorRPCClient(ctx, u, c.ClientConfig)
}
