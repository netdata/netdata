// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRecord(t *testing.T) {
	for name, tc := range map[string]struct {
		line string
		want record
	}{
		"sampled counter": {"requests:2.5|c|#zone:a,path:/api:v1|@.5", record{
			name:   "requests",
			kind:   counter,
			value:  2.5,
			rate:   .5,
			labels: []metrix.Label{{Key: "path", Value: "/api:v1"}, {Key: "zone", Value: "a"}},
		}},
		"gauge assignment": {"level:1e2|g", record{
			name:  "level",
			kind:  gauge,
			value: 100,
			rate:  1,
		}},
		"gauge delta": {"level:-2|g|@1.0", record{
			name:  "level",
			kind:  gauge,
			value: -2,
			rate:  1,
			delta: true,
		}},
		"signed observation": {"difference:-2|h", record{
			name:  "difference",
			kind:  histogram,
			value: -2,
			rate:  1,
		}},
		"zero timer": {"latency:0|ms", record{
			name: "latency",
			kind: timer,
			rate: 1,
		}},
		"exact set member": {"members: zinit:=x,@# y |s", record{
			name:   "members",
			kind:   set,
			member: " zinit:=x,@# y ",
			rate:   1,
		}},
		"duplicate tags collapse": {"x:1|c|#a:b,a:b", record{
			name:   "x",
			kind:   counter,
			value:  1,
			rate:   1,
			labels: []metrix.Label{{Key: "a", Value: "b"}},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := parseRecord(tc.line, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRejectMalformedRecord(t *testing.T) {
	for name, line := range map[string]string{
		"no colon": "x|c", "empty name": ":1|c", "empty value": "x:|g", "prefix numeric": "x:1junk|c",
		"negative counter": "x:-1|c", "negative timer": "x:-1|ms", "nan": "x:NaN|g", "inf": "x:Inf|h",
		"overflow": "x:1e999|h", "hex": "x:0x1p0|c", "underscore": "x:1_000|c", "spaces": "x: 1|c",
		"packed values": "x:1:2|ms", "shorthand": "x:1", "alias": "x:1|m", "empty option": "x:1|c|",
		"zero rate": "x:1|c|@0", "high rate": "x:1|c|@1.01", "duplicate rate": "x:1|c|@1|@1",
		"sampled gauge": "x:1|g|@.5", "sampled set": "x:a|s|@.5", "empty set": "x:|s",
		"conflicting tags": "x:1|c|#a:b,a:c", "bare tag": "x:1|c|#a", "empty tag value": "x:1|c|#a:",
		"empty tag key": "x:1|c|#:b", "empty tags": "x:1|c|#", "duplicate tag sections": "x:1|c|#a:b|#c:d",
		"reserved job": "x:1|c|#_collect_job:x", "control": "x:1\x00|c", "invalid utf8": "x\xff:1|c",
		"embedded newline": "x:1|c\nx:2|c", "embedded cr": "x:1|c\r", "space name": "x y:1|c",
	} {
		t.Run(name, func(t *testing.T) { _, err := parseRecord(line, nil); require.Error(t, err) })
	}
}

func TestFinalPreparation(t *testing.T) {
	for name, tc := range map[string]struct {
		key, value string
		ok         bool
	}{
		"normal": {"pool", "blue", true}, "key boundary": {strings.Repeat("k", 199), "v", true},
		"long key": {strings.Repeat("k", 200), "v", false}, "value boundary": {"k", strings.Repeat("v", 799), true},
		"long value": {"k", strings.Repeat("v", 800), false}, "native punctuation": {"[/key].-", "a:b+c@d(e) /[]", true},
		"unicode value": {"key", "Καλημέρα 世界", true}, "unicode key": {"κ", "v", false},
		"equals": {"k", "a=b", false}, "comma": {"k", "a,b", false}, "quotes": {"k", "a'b", false},
		"tabs": {"k", "a\tb", false}, "spaces": {"k", "a  b", false}, "leading": {"k", " a", false},
		"trailing": {"k", "a ", false}, "all underscore key": {"__", "v", false}, "all underscore value": {"k", "__", false},
		"field reserved": {"measure_field", "min", false}, "job reserved": {"_collect_job", "x", false},
		"le allowed": {"le", "1", true}, "quantile allowed": {"quantile", "0.5", true},
	} {
		t.Run(name, func(t *testing.T) {
			p, err := prepareRecord(
				record{
					name:   "metric",
					kind:   gauge,
					labels: []metrix.Label{{Key: tc.key, Value: tc.value}},
				},
				nil,
				nil,
			)
			if !tc.ok {
				require.ErrorIs(t, err, rejectLabels)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, []metrix.Label{{Key: tc.key, Value: tc.value}}, p.labels)
		})
	}
	for name, tc := range map[string]struct {
		value string
		ok    bool
	}{
		"punctuation": {"Requests = accepted, total (%)", true}, "utf8": {"Μετρήσεις 世界", true},
		"long metadata": {strings.Repeat("x", 2000), true}, "quote": {"user's requests", false},
		"double quote": {"\"x\"", false}, "backslash": {"a\\b", false}, "unicode space": {"\u00a0unit", false},
		"repeated space": {"a  b", false}, "underscore": {"___", false}, "control": {"a\x7fb", false},
	} {
		t.Run("metadata/"+name, func(t *testing.T) {
			p, err := prepareRecord(
				record{
					name:   "metric",
					kind:   gauge,
					labels: []metrix.Label{{Key: "nd_title", Value: tc.value}},
				},
				nil,
				nil,
			)
			if !tc.ok {
				require.ErrorIs(t, err, rejectMetadata)
				return
			}
			require.NoError(t, err)
			assert.Empty(t, p.labels)
			assert.Equal(t, "metric\x00", string(p.id), "metadata is not identity")
			assert.Equal(t, tc.value, p.metadata.title)
		})
	}
	_, err := prepareRecord(record{
		name:   "x",
		labels: []metrix.Label{{Key: "nd_units", Value: "bytes"}},
	}, nil, nil)
	require.ErrorIs(t, err, rejectMetadata)
}

func TestTypedNameEncoding(t *testing.T) {
	assert.Equal(t, "a_5fb.1-_ce_bb", encodeName("a_b.1-λ"))
	seen := make(map[string]string)
	for i := 0; i < 65536; i++ {
		raw := string([]byte{byte(i >> 8), byte(i)})
		encoded := encodeName(raw)
		if old, exists := seen[encoded]; exists {
			t.Fatalf("collision %q and %q", old, raw)
		}
		seen[encoded] = raw
		var decoded strings.Builder
		for j := 0; j < len(encoded); j++ {
			if encoded[j] == '_' {
				b, err := hex.DecodeString(encoded[j+1 : j+3])
				require.NoError(t, err)
				decoded.Write(b)
				j += 2
			} else {
				decoded.WriteByte(encoded[j])
			}
		}
		assert.Equal(t, raw, decoded.String())
	}
	// Raw underscore suffixes cannot alias flattened MeasureSet fields.
	assert.NotEqual(t, "h.values."+encodeName("latency")+"_min", "h.values."+encodeName("latency_min"))
}

func FuzzRecord(f *testing.F) {
	for _, line := range []string{"x:1|c", "x:-1|h|@.3", "members:a:b|s|#a:b", "level:+1|g", "x:NaN|c"} {
		f.Add(line)
	}
	f.Fuzz(func(t *testing.T, line string) {
		r, err := parseRecord(line, nil)
		if err == nil {
			_, _ = prepareRecord(r, nil, nil)
		}
	})
}
