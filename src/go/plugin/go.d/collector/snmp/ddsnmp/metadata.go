// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import "maps"

// MergeMetaTag keeps the first value at equal specificity and lets an exact
// profile value replace a non-exact value.
func MergeMetaTag(dest map[string]MetaTag, key string, tag MetaTag) {
	if existing, ok := dest[key]; !ok || existing.Value == "" || (!existing.IsExactMatch && tag.IsExactMatch) {
		dest[key] = tag
	}
}

// MergeMetaTags merges profile metadata without losing exact-match provenance.
func MergeMetaTags(dest, src map[string]MetaTag) {
	for key, tag := range src {
		MergeMetaTag(dest, key, tag)
	}
}

// ResolveDeviceMetadata applies static fallback, profile metadata, and final
// vnode labels in that order without modifying its inputs.
func ResolveDeviceMetadata(base map[string]string, profile map[string]MetaTag, final map[string]string) map[string]string {
	resolved := make(map[string]string, len(base)+len(profile)+len(final))
	maps.Copy(resolved, base)

	for key, tag := range profile {
		if current, ok := resolved[key]; !ok || current == "" || tag.IsExactMatch {
			resolved[key] = tag.Value
		}
	}
	maps.Copy(resolved, final)

	return resolved
}
