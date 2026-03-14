// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/secretsctl"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

func (m *Manager) dyncfgSecretStoreTemplateID(kind secretstore.StoreKind) string {
	return fmt.Sprintf("%s%s", m.dyncfgSecretStorePrefixValue(), kind)
}

func (m *Manager) dyncfgSecretStoreID(id string) string {
	return fmt.Sprintf("%s%s", m.dyncfgSecretStorePrefixValue(), id)
}

func (m *Manager) secretStoreConfigFromPayload(fn dyncfg.Function, name string, kind secretstore.StoreKind) (secretstore.Config, error) {
	if err := fn.ValidateHasPayload(); err != nil {
		return nil, err
	}

	var payload secretstore.Config
	if err := fn.UnmarshalPayload(&payload); err != nil {
		return nil, fmt.Errorf("invalid configuration format: %w", err)
	}
	if payload == nil {
		payload = secretstore.Config{}
	}
	payload.SetName(name)
	payload.SetKind(kind)
	payload.SetSource(confgroup.TypeDyncfg)
	payload.SetSourceType(confgroup.TypeDyncfg)
	return payload, nil
}

func (m *Manager) lookupSecretStoreEntry(key string) (*dyncfg.Entry[secretstore.Config], bool) {
	entry, ok := m.secretsCtl.Lookup(key)
	return dyncfgSecretStoreTestEntry(entry, ok)
}

func (m *Manager) rememberSecretStoreConfig(cfg secretstore.Config) (*dyncfg.Entry[secretstore.Config], bool, error) {
	entry, changed, err := m.secretsCtl.RememberDiscoveredConfig(cfg)
	if err != nil || !changed {
		return nil, changed, err
	}
	got, _ := dyncfgSecretStoreTestEntry(entry, true)
	return got, true, nil
}

func (m *Manager) removeSecretStoreConfig(cfg secretstore.Config) (*dyncfg.Entry[secretstore.Config], bool) {
	entry, ok := m.secretsCtl.RemoveDiscoveredConfig(cfg)
	return dyncfgSecretStoreTestEntry(entry, ok)
}

func dyncfgSecretStoreTestEntry(entry secretsctl.Entry, ok bool) (*dyncfg.Entry[secretstore.Config], bool) {
	if !ok {
		return nil, false
	}
	return &dyncfg.Entry[secretstore.Config]{
		Cfg:    entry.Cfg,
		Status: entry.Status,
	}, true
}
