// SPDX-License-Identifier: GPL-3.0-or-later

package promyaml

import (
	"strings"
	"testing"
)

type testDocument struct {
	Version string        `yaml:"version"`
	Nested  testNestedMap `yaml:"nested"`
}

type testNestedMap struct {
	Value string `yaml:"value"`
}

func TestDecodeStrictDocument(t *testing.T) {
	tests := map[string]struct {
		content string
		wantErr string
	}{
		"valid": {
			content: "version: v1\nnested:\n  value: ok\n",
		},
		"unknown nested field": {
			content: "version: v1\nnested:\n  value: ok\n  other: no\n",
			wantErr: "field other not found",
		},
		"duplicate root key": {
			content: "version: v1\nversion: v1\nnested:\n  value: ok\n",
			wantErr: "duplicate mapping key",
		},
		"duplicate nested key": {
			content: "version: v1\nnested:\n  value: ok\n  value: again\n",
			wantErr: "duplicate mapping key",
		},
		"anchor": {
			content: "version: v1\nnested: &nested\n  value: ok\n",
			wantErr: "anchors, aliases, and merge keys are not allowed",
		},
		"alias": {
			content: "version: v1\nnested: &nested\n  value: ok\ncopy: *nested\n",
			wantErr: "anchors, aliases, and merge keys are not allowed",
		},
		"merge key": {
			content: "version: v1\nbase: &base\n  value: ok\nnested:\n  <<: *base\n",
			wantErr: "anchors, aliases, and merge keys are not allowed",
		},
		"trailing document": {
			content: "version: v1\nnested:\n  value: ok\n---\n{}\n",
			wantErr: "exactly one YAML document",
		},
		"non mapping document": {
			content: "- version\n- nested\n",
			wantErr: "document must be a YAML mapping",
		},
		"missing required key": {
			content: "version: v1\n",
			wantErr: "required field nested is missing",
		},
		"explicit null": {
			content: "version: v1\nnested: null\n",
			wantErr: "YAML null values are not allowed",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Decode[testDocument](name, []byte(tc.content), "version", "nested")
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Decode() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Decode() error = nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Decode() error = %q, want substring %q", err, tc.wantErr)
			}
		})
	}
}
