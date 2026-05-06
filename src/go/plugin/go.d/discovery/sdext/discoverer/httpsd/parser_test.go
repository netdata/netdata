// SPDX-License-Identifier: GPL-3.0-or-later

package httpsd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseParser_parse(t *testing.T) {
	tests := map[string]struct {
		format      string
		contentType string
		body        string
		want        []any
		wantErr     bool
		wantErrText string
	}{
		"json bare array": {
			format:      formatJSON,
			contentType: "application/json",
			body:        `[{"name":"api","url":"http://127.0.0.1"}, "http://127.0.0.2"]`,
			want: []any{
				map[string]any{"name": "api", "url": "http://127.0.0.1"},
				"http://127.0.0.2",
			},
		},
		"yaml bare array": {
			format: formatYAML,
			body: `
- name: api
  url: http://127.0.0.1
- http://127.0.0.2
`,
			want: []any{
				map[string]any{"name": "api", "url": "http://127.0.0.1"},
				"http://127.0.0.2",
			},
		},
		"json envelope": {
			format: formatJSON,
			body:   `{"items":[{"name":"api"}]}`,
			want:   []any{map[string]any{"name": "api"}},
		},
		"yaml envelope": {
			format: formatYAML,
			body: `
items:
  - name: api
`,
			want: []any{map[string]any{"name": "api"}},
		},
		"auto content type json": {
			format:      formatAuto,
			contentType: "application/vnd.netdata.discovery+json; charset=utf-8",
			body:        `[{"name":"api"}]`,
			want:        []any{map[string]any{"name": "api"}},
		},
		"auto content type yaml": {
			format:      formatAuto,
			contentType: "application/x-yaml",
			body: `
- name: api
`,
			want: []any{map[string]any{"name": "api"}},
		},
		"auto json first without content type": {
			format: formatAuto,
			body:   `[{"name":"api"}]`,
			want:   []any{map[string]any{"name": "api"}},
		},
		"unsupported envelope": {
			format:  formatYAML,
			body:    `jobs: []`,
			wantErr: true,
		},
		"unsupported item type": {
			format:  formatJSON,
			body:    `[1]`,
			wantErr: true,
		},
		"malformed": {
			format:  formatAuto,
			body:    `[`,
			wantErr: true,
		},
		"json trailing garbage": {
			format:      formatJSON,
			body:        `{"items":[]}garbage`,
			wantErr:     true,
			wantErrText: "invalid character",
		},
		"json multiple values": {
			format:      formatJSON,
			body:        `{"items":[]} {"items":[]}`,
			wantErr:     true,
			wantErrText: "multiple JSON values",
		},
		"yaml multiple documents": {
			format: formatYAML,
			body: `
items: []
---
items: []
`,
			wantErr:     true,
			wantErrText: "multiple YAML documents",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			items, err := responseParser{format: tc.format}.parse([]byte(tc.body), tc.contentType)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.wantErrText != "" {
					assert.Contains(t, err.Error(), tc.wantErrText)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, items)
			}
		})
	}
}

func TestTargetsFromItems_HashStability(t *testing.T) {
	items1, err := parseItemsJSON([]byte(`[{"name":"api","url":"http://127.0.0.1","headers":{"b":"2","a":"1"}}]`))
	require.NoError(t, err)
	items2, err := parseItemsJSON([]byte(`[{"headers":{"a":"1","b":"2"},"url":"http://127.0.0.1","name":"api"}]`))
	require.NoError(t, err)

	targets1, err := targetsFromItems("source", items1)
	require.NoError(t, err)
	targets2, err := targetsFromItems("source", items2)
	require.NoError(t, err)

	require.Len(t, targets1, 1)
	require.Len(t, targets2, 1)
	assert.Equal(t, targets1[0].Hash(), targets2[0].Hash())
	assert.Equal(t, targets1[0].TUID(), targets2[0].TUID())
}

func TestTargetsFromItems_ContentChangesHash(t *testing.T) {
	items1, err := parseItemsJSON([]byte(`[{"name":"api","url":"http://127.0.0.1"}]`))
	require.NoError(t, err)
	items2, err := parseItemsJSON([]byte(`[{"name":"api","url":"http://127.0.0.2"}]`))
	require.NoError(t, err)

	targets1, err := targetsFromItems("source", items1)
	require.NoError(t, err)
	targets2, err := targetsFromItems("source", items2)
	require.NoError(t, err)

	require.Len(t, targets1, 1)
	require.Len(t, targets2, 1)
	assert.NotEqual(t, targets1[0].Hash(), targets2[0].Hash())
}

func TestTargetsFromItems_StringItem(t *testing.T) {
	targets, err := targetsFromItems("source", []any{"http://127.0.0.1"})
	require.NoError(t, err)

	require.Len(t, targets, 1)
	tgt := targets[0].(*target)
	assert.Equal(t, "http://127.0.0.1", tgt.Item)
	assert.NotEmpty(t, tgt.TUID())
	assert.Contains(t, tgt.TUID(), "http_item-")
}

func TestSourceString_SanitizesURLAndIsStable(t *testing.T) {
	cfg := Config{}
	cfg.URL = "https://user:pass@example.com/path?token=secret#fragment"
	cfg.Headers = map[string]string{"X-Test": "value"}

	src1 := sourceString(cfg)
	src2 := sourceString(cfg)

	assert.Equal(t, src1, src2)
	assert.Contains(t, src1, "discoverer=http,url=https://example.com/path,hash=")
	assert.NotContains(t, src1, "user")
	assert.NotContains(t, src1, "pass")
	assert.NotContains(t, src1, "token")
	assert.NotContains(t, src1, "secret")
	assert.NotContains(t, src1, "fragment")
}
