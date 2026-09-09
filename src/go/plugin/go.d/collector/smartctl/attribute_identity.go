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

	groups := make(map[string]int)
	for _, attr := range attrs {
		if !isSmartAttrChartable(attr) {
			continue
		}
		baseName := cleanAttributeName(attributeNameMap(attr.name()))
		groups[baseName]++
	}

	identities := make(smartAttributeIdentities)
	for _, attr := range attrs {
		if !isSmartAttrChartable(attr) {
			continue
		}
		baseName := cleanAttributeName(attributeNameMap(attr.name()))
		name := baseName
		if groups[baseName] > 1 {
			name = disambiguateSmartAttributeName(baseName, attr.id())
		}
		identities[attr.id()] = smartAttributeIdentity{baseName: baseName, name: name}
	}
	return identities
}
func disambiguateSmartAttributeName(baseName, id string) string {
	return baseName + "_id_" + cleanAttributeName(id)
}

func isSmartAttrChartable(attr *smartAttribute) bool {
	return isSmartAttrValid(attr) &&
		!strings.HasPrefix(attr.name(), "Unknown") &&
		!strings.HasPrefix(attr.name(), "Not_In_Use")
}
