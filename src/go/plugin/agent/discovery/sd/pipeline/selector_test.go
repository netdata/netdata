// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"

	"github.com/stretchr/testify/assert"
)

var reSrString = regexp.MustCompile(`^{[^{}]+}$`)

func TestSelector_String(t *testing.T) {
	tests := map[string]struct {
		sr            selector
		want          string
		wantRegexForm bool
	}{
		"true selector": {
			sr:   trueSelector{},
			want: "{*}",
		},
		"exact selector": {
			sr:            exactSelector("selector"),
			wantRegexForm: true,
		},
		"neg selector from exact": {
			sr:            negSelector{exactSelector("selector")},
			wantRegexForm: true,
		},
		"neg selector from neg": {
			sr:            negSelector{negSelector{exactSelector("selector")}},
			wantRegexForm: true,
		},
		"neg selector from or": {
			sr: negSelector{orSelector{
				lhs: exactSelector("selector"),
				rhs: exactSelector("selector"),
			}},
			wantRegexForm: true,
		},
		"neg selector from nested or": {
			sr: negSelector{orSelector{
				lhs: orSelector{lhs: exactSelector("selector"), rhs: negSelector{exactSelector("selector")}},
				rhs: orSelector{lhs: exactSelector("selector"), rhs: negSelector{exactSelector("selector")}},
			}},
			wantRegexForm: true,
		},
		"neg selector from nested and": {
			sr: negSelector{andSelector{
				lhs: andSelector{lhs: exactSelector("selector"), rhs: negSelector{exactSelector("selector")}},
				rhs: andSelector{lhs: exactSelector("selector"), rhs: negSelector{exactSelector("selector")}},
			}},
			wantRegexForm: true,
		},
		"or selector": {
			sr: orSelector{
				lhs: orSelector{lhs: exactSelector("selector"), rhs: negSelector{exactSelector("selector")}},
				rhs: orSelector{lhs: exactSelector("selector"), rhs: negSelector{exactSelector("selector")}},
			},
			wantRegexForm: true,
		},
		"and selector": {
			sr: andSelector{
				lhs: andSelector{lhs: exactSelector("selector"), rhs: negSelector{exactSelector("selector")}},
				rhs: andSelector{lhs: exactSelector("selector"), rhs: negSelector{exactSelector("selector")}},
			},
			wantRegexForm: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := fmt.Sprintf("%s", tc.sr)
			if tc.wantRegexForm {
				assert.True(t, reSrString.MatchString(got))
				return
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSelector_Matches(t *testing.T) {
	tests := map[string]struct {
		sr   selector
		tags model.Tags
		want bool
	}{
		"exact selector matches": {
			sr:   exactSelector("a"),
			tags: model.Tags{"a": {}, "b": {}},
			want: true,
		},
		"exact selector does not match": {
			sr:   exactSelector("c"),
			tags: model.Tags{"a": {}, "b": {}},
			want: false,
		},
		"neg selector matches": {
			sr:   negSelector{exactSelector("c")},
			tags: model.Tags{"a": {}, "b": {}},
			want: true,
		},
		"neg selector does not match": {
			sr:   negSelector{exactSelector("a")},
			tags: model.Tags{"a": {}, "b": {}},
			want: false,
		},
		"or selector matches": {
			sr: orSelector{
				lhs: orSelector{lhs: exactSelector("c"), rhs: exactSelector("d")},
				rhs: orSelector{lhs: exactSelector("e"), rhs: exactSelector("b")},
			},
			tags: model.Tags{"a": {}, "b": {}},
			want: true,
		},
		"or selector does not match": {
			sr: orSelector{
				lhs: orSelector{lhs: exactSelector("c"), rhs: exactSelector("d")},
				rhs: orSelector{lhs: exactSelector("e"), rhs: exactSelector("f")},
			},
			tags: model.Tags{"a": {}, "b": {}},
			want: false,
		},
		"and selector matches": {
			sr: andSelector{
				lhs: andSelector{lhs: exactSelector("a"), rhs: exactSelector("b")},
				rhs: andSelector{lhs: exactSelector("c"), rhs: exactSelector("d")},
			},
			tags: model.Tags{"a": {}, "b": {}, "c": {}, "d": {}},
			want: true,
		},
		"and selector does not match": {
			sr: andSelector{
				lhs: andSelector{lhs: exactSelector("a"), rhs: exactSelector("b")},
				rhs: andSelector{lhs: exactSelector("c"), rhs: exactSelector("z")},
			},
			tags: model.Tags{"a": {}, "b": {}, "c": {}, "d": {}},
			want: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.sr.matches(tc.tags))
		})
	}
}

func TestParseSelector(t *testing.T) {
	tests := map[string]struct {
		wantSelector selector
		wantErr      bool
	}{
		"":    {wantSelector: trueSelector{}},
		"a":   {wantSelector: exactSelector("a")},
		"Z":   {wantSelector: exactSelector("Z")},
		"a_b": {wantSelector: exactSelector("a_b")},
		"a=b": {wantSelector: exactSelector("a=b")},
		"!a":  {wantSelector: negSelector{exactSelector("a")}},
		"a b": {wantSelector: andSelector{lhs: exactSelector("a"), rhs: exactSelector("b")}},
		"a|b": {wantSelector: orSelector{lhs: exactSelector("a"), rhs: exactSelector("b")}},
		"*":   {wantSelector: trueSelector{}},
		"!*":  {wantSelector: negSelector{trueSelector{}}},
		"a b !c d|e f": {
			wantSelector: andSelector{
				lhs: andSelector{
					lhs: andSelector{
						lhs: andSelector{lhs: exactSelector("a"), rhs: exactSelector("b")},
						rhs: negSelector{exactSelector("c")},
					},
					rhs: orSelector{
						lhs: exactSelector("d"),
						rhs: exactSelector("e"),
					},
				},
				rhs: exactSelector("f"),
			},
		},
		"!":      {wantErr: true},
		"a !":    {wantErr: true},
		"a!b":    {wantErr: true},
		"0a":     {wantErr: true},
		"a b c*": {wantErr: true},
		"__":     {wantErr: true},
		"a|b|c*": {wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sr, err := parseSelector(name)

			if test.wantErr {
				assert.Nil(t, sr)
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.wantSelector, sr)
			}
		})
	}
}
