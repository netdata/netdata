// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// compileLabelSet validates and canonicalizes labels into a reusable immutable handle.
func (c *storeCore) compileLabelSet(labels ...Label) LabelSet {
	return compileLabelSetForOwner(c, labels...)
}

func compileLabelSetForOwner(owner meterBackend, labels ...Label) LabelSet {
	if len(labels) == 0 {
		return LabelSet{set: &compiledLabelSet{owner: owner}}
	}

	m := make(map[string]string, len(labels))
	for _, l := range labels {
		if l.Key == "" {
			panic(errInvalidLabelKey)
		}
		if _, ok := m[l.Key]; ok {
			panic(errDuplicateLabelKey)
		}
		m[l.Key] = l.Value
	}

	items, key, err := canonicalizeLabels(m)
	if err != nil {
		panic(err)
	}

	return LabelSet{set: &compiledLabelSet{owner: owner, items: items, key: key}}
}
