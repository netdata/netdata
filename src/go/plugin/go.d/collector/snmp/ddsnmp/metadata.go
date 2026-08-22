// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import "maps"

const (
	deviceMetadataVendorKey = "vendor"
	deviceMetadataModelKey  = "model"
)

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

// MergeDeviceIdentityMetadata merges only the fields stored as the shared
// device's structured identity.
func MergeDeviceIdentityMetadata(dest, src map[string]MetaTag) {
	if tag, ok := src[deviceMetadataVendorKey]; ok {
		MergeMetaTag(dest, deviceMetadataVendorKey, tag)
	}
	if tag, ok := src[deviceMetadataModelKey]; ok {
		MergeMetaTag(dest, deviceMetadataModelKey, tag)
	}
}

// ResolveDeviceMetadata applies static fallback, profile metadata, and final
// vnode labels in that order without modifying its inputs.
func ResolveDeviceMetadata(base map[string]string, profile map[string]MetaTag, final map[string]string) map[string]string {
	resolved := make(map[string]string, len(base)+len(profile)+len(final))
	maps.Copy(resolved, base)

	for key, tag := range profile {
		resolved[key] = resolveProfileMetadataValue(resolved[key], tag)
	}
	maps.Copy(resolved, final)

	return resolved
}

// ResolveDeviceIdentity applies metadata precedence to the shared device's
// structured vendor and model without copying unrelated labels.
func ResolveDeviceIdentity(vendor, model string, profile map[string]MetaTag, final map[string]string) (string, string) {
	vendor = resolveDeviceMetadataValue(deviceMetadataVendorKey, vendor, profile, final)
	model = resolveDeviceMetadataValue(deviceMetadataModelKey, model, profile, final)
	return vendor, model
}

func resolveDeviceMetadataValue(key, base string, profile map[string]MetaTag, final map[string]string) string {
	if tag, ok := profile[key]; ok {
		base = resolveProfileMetadataValue(base, tag)
	}
	if value, ok := final[key]; ok {
		base = value
	}
	return base
}

func resolveProfileMetadataValue(base string, profile MetaTag) string {
	if base == "" || profile.IsExactMatch {
		return profile.Value
	}
	return base
}
