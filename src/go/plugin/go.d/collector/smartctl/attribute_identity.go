// SPDX-License-Identifier: GPL-3.0-or-later

package smartctl

import "strings"

type smartAttributeIdentity struct {
	baseName string
	name     string
}

type smartAttributeIdentities map[string]smartAttributeIdentity

func newSmartAttributeIdentities(dev *smartDevice) smartAttributeIdentities {
	attrs, ok := dev.ataSmartAttributeTable()
	if !ok {
		return nil
	}

	type attributeGroup struct {
		count        int
		hasChartable bool
	}
	groups := make(map[string]attributeGroup)
	for _, attr := range attrs {
		if !isSmartAttrValid(attr) {
			continue
		}
		baseName := cleanAttributeName(attributeNameMap(attr.name()))
		group := groups[baseName]
		group.count++
		group.hasChartable = group.hasChartable || isSmartAttrChartable(attr)
		groups[baseName] = group
	}

	identities := make(smartAttributeIdentities)
	for _, attr := range attrs {
		if !isSmartAttrValid(attr) {
			continue
		}
		baseName := cleanAttributeName(attributeNameMap(attr.name()))
		name := baseName
		if group := groups[baseName]; group.count > 1 && group.hasChartable {
			name = disambiguateSmartAttributeName(baseName, attr.id())
		}
		identities[attr.id()] = smartAttributeIdentity{baseName: baseName, name: name}
	}
	return identities
}

func (ids smartAttributeIdentities) resolve(attr *smartAttribute) smartAttributeIdentity {
	if identity, ok := ids[attr.id()]; ok {
		return identity
	}

	baseName := cleanAttributeName(attributeNameMap(attr.name()))
	name := baseName
	for id, identity := range ids {
		if id != attr.id() && identity.baseName == baseName {
			name = disambiguateSmartAttributeName(baseName, attr.id())
			break
		}
	}
	return smartAttributeIdentity{baseName: baseName, name: name}
}

func disambiguateSmartAttributeName(baseName, id string) string {
	return baseName + "_id_" + cleanAttributeName(id)
}

func isSmartAttrChartable(attr *smartAttribute) bool {
	return isSmartAttrValid(attr) &&
		!strings.HasPrefix(attr.name(), "Unknown") &&
		!strings.HasPrefix(attr.name(), "Not_In_Use")
}
