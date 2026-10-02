package ddprofiledefinition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_IsPlainOid(t *testing.T) {
	tests := map[string]struct {
		in   string
		want bool
	}{
		"plain":                    {"1.3.6.1.4.1.9.1.217", true},
		"single number":            {"1", true},
		"empty":                    {"", false},
		"trailing dot":             {"1.3.6.", false}, // not plain by our regex
		"wildcard star":            {"1.3.6.*", false},
		"regex meta":               {"1.3.6.1.4.1.9.1.21.*", false},
		"letters invalid":          {"1.3.abc", false},
		"double dots invalid":      {"1..3.6", false},
		"leading dot invalid":      {".1.3.6", false},
		"trailing garbage invalid": {"1.3.6. ", false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := IsPlainOid(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_SelectorOidMatches(t *testing.T) {
	tests := map[string]struct {
		device string
		sel    string
		want   bool
	}{
		"exact match plain":           {"1.3.6.1", "1.3.6.1", true},
		"exact mismatch plain":        {"1.3.6.1", "1.3.6.2", false},
		"regex star behaves as regex": {"1.3.6.123", "1.3.6.*", true},
		"regex dot wildcard":          {"1a3a6aZ", "1.3.6.*", true},
		"regex invalid -> false":      {"1.3.6.1", "1.3.6.(", false},
		"regex alternative":           {"1.3.6.9", "1\\.3\\.6\\.(9|10)", true},
		"regex alternative miss":      {"1.3.6.11", "1\\.3\\.6\\.(9|10)", false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := SelectorOidMatches(tt.device, tt.sel)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_SelectorSpec_HasExactOidMatch(t *testing.T) {
	tests := map[string]struct {
		spec SelectorSpec
		oid  string
		want bool
	}{
		"single exact match": {
			spec: SelectorSpec{
				{SysObjectID: SelectorIncludeExclude{
					Include: []string{"1.3.6.1"},
				}},
			},
			oid:  "1.3.6.1",
			want: true,
		},
		"exact not present": {
			spec: SelectorSpec{
				{SysObjectID: SelectorIncludeExclude{
					Include: []string{"1.3.6.2"},
				}},
			},
			oid:  "1.3.6.1",
			want: false,
		},
		"regex present but no exact": {
			spec: SelectorSpec{
				{SysObjectID: SelectorIncludeExclude{
					Include: []string{"1.3.6.*"},
				}},
			},
			oid:  "1.3.6.1",
			want: false, // only counts literal equality
		},
		"mixed regex + exact": {
			spec: SelectorSpec{
				{SysObjectID: SelectorIncludeExclude{
					Include: []string{"1.3.6.*", "1.3.6.1"},
				}},
			},
			oid:  "1.3.6.1",
			want: true,
		},
		"multiple rules one exact": {
			spec: SelectorSpec{
				{SysObjectID: SelectorIncludeExclude{
					Include: []string{"1.3.6.*"},
				}},
				{SysObjectID: SelectorIncludeExclude{
					Include: []string{"1.3.6.1"},
				}},
			},
			oid:  "1.3.6.1",
			want: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := tt.spec.HasExactOidMatch(tt.oid)
			assert.Equal(t, tt.want, got)
		})
	}
}

type selectorMatchResult struct {
	matched bool
	prefix  string
}

func Test_SelectorSpec_Matches(t *testing.T) {
	tests := map[string]struct {
		spec        SelectorSpec
		deviceOID   string
		deviceDescr string
		want        selectorMatchResult
	}{
		"no rules -> no match": {
			spec:      SelectorSpec{},
			deviceOID: "1.3.6.1",
			want: selectorMatchResult{
				matched: false,
				prefix:  "",
			},
		},
		"OID include regex only -> matches": {
			spec: SelectorSpec{
				{SysObjectID: SelectorIncludeExclude{
					Include: []string{"1.3.6.*"},
				}},
			},
			deviceOID: "1.3.6.9",
			want: selectorMatchResult{
				matched: true,
				prefix:  "1.3.6.*",
			}, // longest (only) selector that matched
		},
		"OID include exact found first -> early break returns exact": {
			spec: SelectorSpec{
				{SysObjectID: SelectorIncludeExclude{
					Include: []string{"1.3.6.9", "1.3.6.*"},
				}},
			},
			deviceOID: "1.3.6.9",
			want: selectorMatchResult{
				matched: true,
				prefix:  "1.3.6.9",
			}, // exact wins (break when IsPlainOid)
		},
		"OID include none -> no match": {
			spec: SelectorSpec{
				{SysObjectID: SelectorIncludeExclude{
					Include: []string{"1.3.7.*"},
				}},
			},
			deviceOID: "1.3.6.9",
			want: selectorMatchResult{
				matched: false,
				prefix:  "",
			},
		},
		"OID exclude blocks": {
			spec: SelectorSpec{
				{SysObjectID: SelectorIncludeExclude{
					Include: []string{"1.3.6.*"},
					Exclude: []string{"1.3.6.9"},
				}},
			},
			deviceOID: "1.3.6.9",
			want: selectorMatchResult{
				matched: false,
				prefix:  "",
			},
		},
		"sysDescr include passes (no OID include)": {
			spec: SelectorSpec{
				{SysDescr: SelectorIncludeExclude{
					Include: []string{"RouterOS"},
				}},
			},
			deviceOID:   "0.0", // irrelevant
			deviceDescr: "MikroTik RouterOS v7.10",
			want: selectorMatchResult{
				matched: true,
				prefix:  "",
			}, // no OID include -> empty prefix
		},
		"sysDescr exclude blocks": {
			spec: SelectorSpec{
				{SysDescr: SelectorIncludeExclude{
					Include: []string{"RouterOS"},
					Exclude: []string{"Not some pulse"},
				}},
			},
			deviceOID:   "0.0",
			deviceDescr: "MikroTik RouterOS v7.10 (Not some pulse string)",
			want: selectorMatchResult{
				matched: false,
				prefix:  "",
			},
		},
		"both OID and sysDescr required when both present": {
			spec: SelectorSpec{
				{SysObjectID: SelectorIncludeExclude{
					Include: []string{"1.3.6.*"},
				},
					SysDescr: SelectorIncludeExclude{
						Include: []string{"IOS"},
					}},
			},
			deviceOID:   "1.3.6.9",
			deviceDescr: "Cisco IOS XE",
			want: selectorMatchResult{
				matched: true,
				prefix:  "1.3.6.*",
			},
		},
		"sysDescr include fails -> overall no match": {
			spec: SelectorSpec{
				{SysObjectID: SelectorIncludeExclude{
					Include: []string{"1.3.6.*"},
				},
					SysDescr: SelectorIncludeExclude{
						Include: []string{"JunOS"},
					}},
			},
			deviceOID:   "1.3.6.9",
			deviceDescr: "Cisco IOS XE",
			want: selectorMatchResult{
				matched: false,
				prefix:  "",
			},
		},
		"longest include wins when multiple regex includes match": {
			spec: SelectorSpec{
				{SysObjectID: SelectorIncludeExclude{
					Include: []string{"1.3.*", "1.3.6.*"},
				}},
			},
			deviceOID: "1.3.6.9",
			want: selectorMatchResult{
				matched: true,
				prefix:  "1.3.6.*",
			}, // longer string wins per code
		},
		"multiple rules: first matching rule returns": {
			spec: SelectorSpec{
				{SysObjectID: SelectorIncludeExclude{
					Include: []string{"2.2.*"},
				}},
				{SysObjectID: SelectorIncludeExclude{
					Include: []string{"1.3.6.*"},
				}},
			},
			deviceOID: "1.3.6.9",
			want: selectorMatchResult{
				matched: true,
				prefix:  "1.3.6.*",
			},
		},
		"regex invalid in exclude -> treated as not matched, rule continues": {
			spec: SelectorSpec{
				{SysObjectID: SelectorIncludeExclude{
					Include: []string{"1.3.6.*"},
					Exclude: []string{"1.3.6.("}, // invalid regex: MatchString error -> false
				}},
			},
			deviceOID: "1.3.6.9",
			want: selectorMatchResult{
				matched: true,
				prefix:  "1.3.6.*",
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ok, prefix := tt.spec.Matches(tt.deviceOID, tt.deviceDescr)
			assert.Equal(t, tt.want, selectorMatchResult{
				matched: ok,
				prefix:  prefix,
			})
		})
	}
}

func Test_SelectorSpec_Clone(t *testing.T) {
	tests := map[string]struct {
		newSelector  func() SelectorSpec
		mutate       func(SelectorSpec)
		wantMatch    selectorMatchResult
		wantNilClone bool
	}{
		"nil":   {newSelector: func() SelectorSpec { return nil }, wantNilClone: true},
		"empty": {newSelector: func() SelectorSpec { return SelectorSpec{} }, wantNilClone: true},
		"populated": {
			newSelector: func() SelectorSpec {
				return SelectorSpec{
					{
						SysObjectID: SelectorIncludeExclude{
							Include: []string{"1.3.6.*", "1.3.6.9"},
							Exclude: []string{"2.2.*"},
						},
						SysDescr: SelectorIncludeExclude{
							Include: []string{"RouterOS", "IOS"},
							Exclude: []string{"Not some pulse"},
						},
					},
				}
			},
			mutate: func(cloned SelectorSpec) {
				cloned[0].SysObjectID.Include[0] = "CHANGED"
				cloned[0].SysObjectID.Exclude = append(cloned[0].SysObjectID.Exclude, "extra")
				cloned[0].SysDescr.Include[1] = "CHANGED2"
			},
			wantMatch: selectorMatchResult{
				matched: true,
				prefix:  "1.3.6.*",
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			original, want := tc.newSelector(), tc.newSelector()
			cloned := original.Clone()
			wantClone := tc.newSelector()
			if tc.wantNilClone {
				wantClone = nil
			}
			require.Equal(t, wantClone, cloned)
			if tc.mutate != nil {
				tc.mutate(cloned)
				assert.NotEqual(t, want, cloned)
			}
			assert.Equal(t, want, original)
			matched, prefix := original.Matches("1.3.6.9", "MikroTik RouterOS v7")
			assert.Equal(t, tc.wantMatch, selectorMatchResult{
				matched: matched,
				prefix:  prefix,
			})
		})
	}
}
