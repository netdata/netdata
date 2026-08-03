// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"

type VarbindDef = catalog.VarbindDef
type TrapDef = catalog.TrapDef
type ProfileIndex = catalog.Epoch

const maxMessageLen = catalog.MaxMessageLen

func renderMessage(entry *TrapEntry, td *TrapDef) string { return catalog.RenderMessage(entry, td) }
func renderLabels(entry *TrapEntry, td *TrapDef) map[string]string {
	return catalog.RenderLabels(entry, td)
}
func trapEntryHasUnresolvedTemplate(entry *TrapEntry) bool {
	return catalog.EntryHasUnresolvedTemplate(entry)
}
func truncateUTF8(s string, maxBytes int) string { return catalog.TruncateUTF8(s, maxBytes) }
func resolve2TierVarbind(oid string, raw VarbindValue, td *TrapDef) VarbindValue {
	return catalog.ResolveVarbind(oid, raw, td)
}

// Root packet rendering and dedup retain these catalog bridges until their
// approved PR9/PR10 ownership moves.
func newProfileIndex() *ProfileIndex { return catalog.NewEpoch() }
func prepareTrapDefinition(td *TrapDef) error {
	return catalog.PrepareTrap(td)
}
