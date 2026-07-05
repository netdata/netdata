// SPDX-License-Identifier: GPL-3.0-or-later

package secretsctl

import (
	"context"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

func (c *Controller) publishInitialConfig(rawCfg secretstore.Config) {
	cfg, prepErr := c.prepareConfigCandidate(rawCfg)
	c.handler.RememberDiscoveredConfig(cfg)

	if existing, ok := c.lookupInternal(cfg.ExposedKey()); ok {
		if shouldKeepExisting(cfg, existing) {
			return
		}
		if existing.Status == dyncfg.StatusRunning || existing.Status == dyncfg.StatusFailed {
			// Boot publishing carries no command run: no restart stage, no
			// terminal message.
			c.cb.Stop(context.Background(), existing.Cfg)
		}
	}

	entry := &dyncfg.Entry[secretstore.Config]{Cfg: cfg, Status: dyncfg.StatusFailed}
	if prepErr == nil {
		if err := c.cb.Start(context.Background(), cfg); err == nil {
			entry.Status = dyncfg.StatusRunning
		}
	}

	c.exposed.Add(entry)
	c.handler.NotifyConfigCreate(cfg, entry.Status)
}

func (c *Controller) prepareConfigCandidate(cfg secretstore.Config) (secretstore.Config, error) {
	return cfg, c.validateConfig(context.Background(), cfg)
}

func shouldKeepExisting(cfg secretstore.Config, existing *dyncfg.Entry[secretstore.Config]) bool {
	sp, ep := cfg.SourceTypePriority(), existing.Cfg.SourceTypePriority()
	if ep > sp {
		return true
	}
	if ep < sp {
		return false
	}

	return existing.Status == dyncfg.StatusRunning
}
