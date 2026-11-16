// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeMetaTags(t *testing.T) {
	tests := map[string]struct {
		dest map[string]MetaTag
		src  map[string]MetaTag
		want map[string]MetaTag
	}{
		"fills missing and empty values": {
			dest: map[string]MetaTag{
				"vendor": {Value: ""},
			},
			src: map[string]MetaTag{
				"vendor": {Value: "profile-vendor"},
				"model":  {Value: "profile-model"},
			},
			want: map[string]MetaTag{
				"vendor": {Value: "profile-vendor"},
				"model":  {Value: "profile-model"},
			},
		},
		"exact value replaces non-exact value": {
			dest: map[string]MetaTag{
				"model": {Value: "wildcard-model"},
			},
			src: map[string]MetaTag{
				"model": {Value: "exact-model", IsExactMatch: true},
			},
			want: map[string]MetaTag{
				"model": {Value: "exact-model", IsExactMatch: true},
			},
		},
		"first value wins at equal specificity": {
			dest: map[string]MetaTag{
				"vendor": {Value: "first-vendor", IsExactMatch: true},
			},
			src: map[string]MetaTag{
				"vendor": {Value: "second-vendor", IsExactMatch: true},
			},
			want: map[string]MetaTag{
				"vendor": {Value: "first-vendor", IsExactMatch: true},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			MergeMetaTags(tc.dest, tc.src)
			require.Equal(t, tc.want, tc.dest)
		})
	}
}

func TestMergeDeviceIdentityMetadata(t *testing.T) {
	dest := map[string]MetaTag{
		"vendor": {Value: "wildcard-vendor"},
	}
	src := map[string]MetaTag{
		"vendor":        {Value: "exact-vendor", IsExactMatch: true},
		"model":         {Value: "profile-model"},
		"serial_number": {Value: "ignored-serial", IsExactMatch: true},
	}

	MergeDeviceIdentityMetadata(dest, src)

	require.Equal(t, map[string]MetaTag{
		"vendor": {Value: "exact-vendor", IsExactMatch: true},
		"model":  {Value: "profile-model"},
	}, dest)
}

func TestResolveDeviceMetadata(t *testing.T) {
	base := map[string]string{
		"vendor": "static-vendor",
		"model":  "static-model",
		"type":   "static-type",
	}
	profile := map[string]MetaTag{
		"vendor": {Value: "wildcard-vendor"},
		"model":  {Value: "exact-model", IsExactMatch: true},
		"serial": {Value: "profile-serial"},
	}
	final := map[string]string{
		"model": "operator-model",
		"type":  "operator-type",
	}

	baseBefore := maps.Clone(base)
	profileBefore := maps.Clone(profile)
	finalBefore := maps.Clone(final)

	resolved := ResolveDeviceMetadata(base, profile, final)

	require.Equal(t, map[string]string{
		"vendor": "static-vendor",
		"model":  "operator-model",
		"type":   "operator-type",
		"serial": "profile-serial",
	}, resolved)
	require.Equal(t, baseBefore, base)
	require.Equal(t, profileBefore, profile)
	require.Equal(t, finalBefore, final)
}

func TestResolveDeviceMetadataKeepsSystemSysObjectID(t *testing.T) {
	tests := map[string]struct {
		base    map[string]string
		profile map[string]MetaTag
		final   map[string]string
		want    map[string]string
	}{
		"rejected system value is not filled by profile": {
			base:    map[string]string{"sys_object_id": ""},
			profile: map[string]MetaTag{"sys_object_id": {Value: "private device identifier"}},
			want:    map[string]string{"sys_object_id": ""},
		},
		"exact profile does not replace normalized system value": {
			base:    map[string]string{"sys_object_id": "1.3.6.1.4.1.9.1.1166"},
			profile: map[string]MetaTag{"sys_object_id": {Value: " .1.3.6.1.4.1.9.1.1166\n", IsExactMatch: true}},
			want:    map[string]string{"sys_object_id": "1.3.6.1.4.1.9.1.1166"},
		},
		"final label still overrides": {
			base:    map[string]string{"sys_object_id": "1.3.6.1.4.1.9.1.1166"},
			profile: map[string]MetaTag{"sys_object_id": {Value: "1.3.6.1.4.1.9.1.1166", IsExactMatch: true}},
			final:   map[string]string{"sys_object_id": "operator-value"},
			want:    map[string]string{"sys_object_id": "operator-value"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, ResolveDeviceMetadata(tc.base, tc.profile, tc.final))
		})
	}
}

func TestResolveDeviceMetadataFinalEmptyValueRemainsAuthoritative(t *testing.T) {
	resolved := ResolveDeviceMetadata(
		map[string]string{"model": "static-model"},
		map[string]MetaTag{"model": {Value: "profile-model", IsExactMatch: true}},
		map[string]string{"model": ""},
	)

	value, ok := resolved["model"]
	require.True(t, ok)
	require.Empty(t, value)
}

func TestResolveDeviceIdentity(t *testing.T) {
	vendor, model := ResolveDeviceIdentity(
		"static-vendor",
		"static-model",
		map[string]MetaTag{
			"vendor":        {Value: "wildcard-vendor"},
			"model":         {Value: "exact-model", IsExactMatch: true},
			"serial_number": {Value: "ignored-serial", IsExactMatch: true},
		},
		map[string]string{
			"model":         "final-model",
			"serial_number": "ignored-final-serial",
		},
	)

	require.Equal(t, "static-vendor", vendor)
	require.Equal(t, "final-model", model)
}
