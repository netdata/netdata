// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"encoding/json"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

func snmpConfig(t *testing.T, name, address string) *vnodes.Config {
	t.Helper()
	var c vnodes.Config
	require.NoError(t, json.Unmarshal([]byte(`{"name":"`+name+`","mode":"snmp","mode_snmp":{"address":"`+address+`","credentials":{"community":"fixture"}},"labels":{"site":"override"}}`), &c))
	c.SourceType = "dyncfg"
	c.Source = "user=test"
	require.NoError(t, c.Validate())
	return &c
}
func commitConfig(t *testing.T, vc *VNodeConfiguration, c *vnodes.Config) {
	t.Helper()
	current, _ := vc.Lookup(c.Name)
	p, err := vc.PrepareUpsert(c.Name, current.Revision, c)
	require.NoError(t, err)
	_, err = p.Commit()
	require.NoError(t, err)
}
func publishMetadata(t *testing.T, vc *VNodeConfiguration, token *AcquisitionToken, m *vnodes.Metadata, failed bool) {
	t.Helper()
	p, err := vc.PrepareMetadata(token, m, failed)
	require.NoError(t, err)
	_, err = p.Commit()
	require.NoError(t, err)
}
func TestSNMPAcquisitionUsesCurrentOverridesAndRetainsLastGood(t *testing.T) {
	vc := newTestVNodeConfiguration(t)
	c := snmpConfig(t, "router", "device")
	commitConfig(t, vc, c)
	pending, ok := vc.Lookup("router")
	require.True(t, ok)
	require.Nil(t, pending.Vnode)
	token, _ := vc.Acquisition("router")
	c.Labels["site"] = "new override"
	commitConfig(t, vc, c)
	unchanged, _ := vc.Acquisition("router")
	require.Same(t, token, unchanged)
	raw := &vnodes.Metadata{Hostname: "device-name", Labels: map[string]string{"site": "acquired", "serial": "first"}}
	publishMetadata(t, vc, token, raw, true)
	first, _ := vc.Lookup("router")
	require.Equal(t, "new override", first.Vnode.Labels["site"])
	require.Equal(t, "device-name", first.Vnode.Hostname)
	raw.Labels["serial"] = "mutated"
	again, _ := vc.Lookup("router")
	require.Equal(t, "first", again.Vnode.Labels["serial"])
	publishMetadata(t, vc, token, &vnodes.Metadata{Hostname: "degraded", Labels: map[string]string{}}, true)
	retained, _ := vc.Lookup("router")
	require.Equal(t, first.Vnode, retained.Vnode)
	delete(c.Labels, "site")
	commitConfig(t, vc, c)
	withoutOverride, _ := vc.Lookup("router")
	require.Equal(t, "acquired", withoutOverride.Vnode.Labels["site"])
	c.ModeSNMP.Credentials.Community = "rotated"
	commitConfig(t, vc, c)
	rotated, _ := vc.Acquisition("router")
	require.NotSame(t, token, rotated)
	_, err := vc.PrepareMetadata(token, raw, false)
	require.ErrorIs(t, err, ErrVNodeRevision)
	publishMetadata(t, vc, rotated, nil, true)
	kept, _ := vc.Lookup("router")
	require.Equal(t, "first", kept.Vnode.Labels["serial"])
	publishMetadata(t, vc, rotated, &vnodes.Metadata{Hostname: "new-name", Labels: map[string]string{"serial": "second"}}, false)
	updated, _ := vc.Lookup("router")
	require.Equal(t, "new-name", updated.Vnode.Hostname)
	require.NotContains(t, updated.Vnode.Labels, "site")
	authored, _ := vc.Authored("router")
	require.Empty(t, authored.Config.Hostname)
	require.False(t, authored.Failed)
}
func TestSNMPRemoveReaddFencesPreparedAndAcquiredResults(t *testing.T) {
	vc := newTestVNodeConfiguration(t)
	c := snmpConfig(t, "router", "device")
	commitConfig(t, vc, c)
	token, _ := vc.Acquisition("router")
	late, err := vc.PrepareMetadata(token, &vnodes.Metadata{Hostname: "old"}, false)
	require.NoError(t, err)
	old, _ := vc.Lookup("router")
	removal, err := vc.PrepareRemove("router", old.Revision)
	require.NoError(t, err)
	_, err = removal.Commit()
	require.NoError(t, err)
	commitConfig(t, vc, c)
	_, err = late.Commit()
	require.ErrorIs(t, err, ErrVNodeRevision)
	_, err = vc.PrepareMetadata(token, &vnodes.Metadata{Hostname: "old"}, false)
	require.ErrorIs(t, err, ErrVNodeRevision)
	current, _ := vc.Lookup("router")
	require.Nil(t, current.Vnode)
}
func TestSNMPReservesIdentityBeforeReadinessAndRejectsHostnameCollision(t *testing.T) {
	vc := newTestVNodeConfiguration(t)
	a := snmpConfig(t, "a", "device-a")
	b := snmpConfig(t, "b", "device-b")
	commitConfig(t, vc, a)
	same := snmpConfig(t, "same", "device-a")
	_, err := vc.PrepareUpsert("same", 0, same)
	require.ErrorContains(t, err, "duplicate")
	commitConfig(t, vc, b)
	ta, _ := vc.Acquisition("a")
	tb, _ := vc.Acquisition("b")
	publishMetadata(t, vc, ta, &vnodes.Metadata{Hostname: "shared-name"}, false)
	publishMetadata(t, vc, tb, &vnodes.Metadata{Hostname: "shared-name"}, false)
	pending, _ := vc.Authored("b")
	require.Nil(t, pending.Snapshot.Vnode)
	require.True(t, pending.Failed)
	publishMetadata(t, vc, tb, &vnodes.Metadata{Hostname: "unique-name"}, false)
	ready, _ := vc.Authored("b")
	require.NotNil(t, ready.Snapshot.Vnode)
	require.False(t, ready.Failed)
}
