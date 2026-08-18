// SPDX-License-Identifier: GPL-3.0-or-later

package projector

type bridgeVLANAliasIndex struct {
	rawScopeByAlias map[string]string
	ambiguous       map[string]struct{}
}

func buildBridgeVLANAliasIndex(links []bridgeMacLinkRecord) bridgeVLANAliasIndex {
	index := bridgeVLANAliasIndex{
		rawScopeByAlias: make(map[string]string),
		ambiguous:       make(map[string]struct{}),
	}
	for _, link := range links {
		aliasKey := bridgePortVLANAliasKey(link.port)
		rawScope := bridgePortForwardingDomain(link.port)
		if aliasKey == "" || rawScope == "" || rawScope == bridgePortVLANScope(link.port) {
			continue
		}
		if _, ambiguous := index.ambiguous[aliasKey]; ambiguous {
			continue
		}
		if existing := index.rawScopeByAlias[aliasKey]; existing != "" && existing != rawScope {
			delete(index.rawScopeByAlias, aliasKey)
			index.ambiguous[aliasKey] = struct{}{}
			continue
		}
		index.rawScopeByAlias[aliasKey] = rawScope
	}
	return index
}

func (i bridgeVLANAliasIndex) uniqueAliasKey(port bridgePortRef) string {
	aliasKey := bridgePortVLANAliasKey(port)
	if aliasKey == "" {
		return ""
	}
	if _, ambiguous := i.ambiguous[aliasKey]; ambiguous {
		return ""
	}
	if i.rawScopeByAlias[aliasKey] != bridgePortForwardingDomain(port) {
		return ""
	}
	return aliasKey
}

func bridgePortVLANAliasKey(port bridgePortRef) string {
	identity := bridgePortCanonicalIdentity(port)
	vlanScope := bridgePortVLANScope(port)
	if identity == "" || vlanScope == "" {
		return ""
	}
	return identity + keySep + "scope:" + vlanScope
}
