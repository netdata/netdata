// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidHTTPSURL(t *testing.T) {
	tests := map[string]struct {
		address string
		valid   bool
	}{
		"https with host":  {address: "https://artifacts.example.invalid/runtime.tar.zst", valid: true},
		"query allowed":    {address: "https://artifacts.example.invalid/runtime?mirror=1", valid: true},
		"plain http":       {address: "http://artifacts.example.invalid/runtime.tar.zst"},
		"missing host":     {address: "https:///runtime.tar.zst"},
		"relative":         {address: "/runtime.tar.zst"},
		"credentials":      {address: "https://user:secret@artifacts.example.invalid/runtime.tar.zst"},
		"user only":        {address: "https://user@artifacts.example.invalid/runtime.tar.zst"},
		"fragment":         {address: "https://artifacts.example.invalid/runtime.tar.zst#part"},
		"unparsable":       {address: "https://artifacts.example.invalid/%zz"},
		"empty":            {address: ""},
		"uppercase scheme": {address: "HTTPS://artifacts.example.invalid/runtime.tar.zst", valid: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.valid, ValidHTTPSURL(test.address))
		})
	}
}

func TestManifestEqualityPolicies(t *testing.T) {
	base, _ := bundleFixture(t, "equality")
	mirrored := base
	mirrored.Archive.URL = "https://mirror.example.invalid/runtime.tar.zst"
	rebound := base
	rebound.Bindings = append([]Binding(nil), base.Bindings...)
	rebound.Bindings[0].Offset++
	tests := map[string]struct {
		other      Manifest
		manifest   bool
		executable bool
	}{
		"identical":           {other: base, manifest: true, executable: true},
		"archive URL differs": {other: mirrored, executable: true},
		"binding differs":     {other: rebound},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.manifest, sameManifest(base, test.other))
			assert.Equal(t, test.executable, sameExecutable(base, test.other))
		})
	}
}
