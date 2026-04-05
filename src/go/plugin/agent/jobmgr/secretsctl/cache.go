// SPDX-License-Identifier: GPL-3.0-or-later

package secretsctl

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

func (c *Controller) Lookup(key string) (Entry, bool) {
	entry, ok := c.lookup(key)
	if !ok {
		return Entry{}, false
	}
	return entry, true
}

func (c *Controller) lookup(key string) (Entry, bool) {
	entry, ok := c.lookupInternal(key)
	if !ok {
		return Entry{}, false
	}
	return entryFromDyncfg(entry), true
}

func (c *Controller) lookupInternal(key string) (*dyncfg.Entry[secretstore.Config], bool) {
	if strings.TrimSpace(key) == "" || c.exposed == nil {
		return nil, false
	}
	return c.exposed.LookupByKey(key)
}

func (c *Controller) RememberDiscoveredConfig(cfg secretstore.Config) (Entry, bool, error) {
	entry, changed, err := c.rememberDiscoveredConfig(cfg)
	if err != nil || !changed || entry == nil {
		return Entry{}, changed, err
	}
	return entryFromDyncfg(entry), true, nil
}

func (c *Controller) rememberDiscoveredConfig(cfg secretstore.Config) (*dyncfg.Entry[secretstore.Config], bool, error) {
	if err := c.validateConfig(cfg); err != nil {
		return nil, false, err
	}

	c.handler.RememberDiscoveredConfig(cfg)

	entry, ok := c.lookupInternal(cfg.ExposedKey())
	if !ok {
		entry = c.handler.AddDiscoveredConfig(cfg, dyncfg.StatusAccepted)
		c.handler.NotifyJobCreate(cfg, dyncfg.StatusAccepted)
		return entry, true, nil
	}

	sp, ep := cfg.SourceTypePriority(), entry.Cfg.SourceTypePriority()
	if ep > sp || (ep == sp && entry.Status == dyncfg.StatusRunning) {
		return entry, false, nil
	}

	if entry.Status == dyncfg.StatusRunning || entry.Status == dyncfg.StatusFailed {
		c.cb.Stop(entry.Cfg)
		c.cb.TakeCommandMessage()
	}

	entry = c.handler.AddDiscoveredConfig(cfg, dyncfg.StatusAccepted)
	c.handler.NotifyJobCreate(cfg, dyncfg.StatusAccepted)
	return entry, true, nil
}

func (c *Controller) RemoveDiscoveredConfig(cfg secretstore.Config) (Entry, bool) {
	entry, ok := c.removeDiscoveredConfig(cfg)
	if !ok || entry == nil {
		return Entry{}, false
	}
	return entryFromDyncfg(entry), true
}

func (c *Controller) removeDiscoveredConfig(cfg secretstore.Config) (*dyncfg.Entry[secretstore.Config], bool) {
	entry, ok := c.handler.RemoveDiscoveredConfig(cfg)
	if !ok {
		return nil, false
	}

	c.cb.Stop(entry.Cfg)
	c.cb.TakeCommandMessage()
	c.handler.NotifyJobRemove(entry.Cfg)
	return entry, true
}

func (c *Controller) validateConfig(cfg secretstore.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if c.service == nil {
		return fmt.Errorf("secretstore service is not available")
	}
	return c.service.Validate(secretStoreResolveContext(c.Logger, cfg), cfg)
}

func (c *Controller) validateStored(key string) error {
	entry, ok := c.lookupInternal(key)
	if !ok {
		return secretstore.ErrStoreNotFound
	}
	if c.service == nil {
		return fmt.Errorf("secretstore service is not available")
	}

	if _, ok := c.service.GetStatus(key); ok {
		return c.service.ValidateStored(secretStoreResolveContextForKey(c.Logger, key, entry.Cfg.Kind(), entry.Cfg.Name()), key)
	}

	return c.validateConfig(entry.Cfg)
}

func (c *Controller) affectedJobsFor(storeKey string) []secretstore.JobRef {
	if c.affectedJobs == nil {
		return nil
	}
	return c.affectedJobs(storeKey)
}

func (c *Controller) restartableAffectedJobsFor(storeKey string) []secretstore.JobRef {
	if c.restartableAffectedJobs == nil {
		return nil
	}
	return c.restartableAffectedJobs(storeKey)
}
