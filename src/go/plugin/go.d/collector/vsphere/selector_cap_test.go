// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

type includeSelectorTestCase struct {
	include []string
	want    int
}

func prefixedSelectorCases(kind, prefix, match, missing string) map[string]includeSelectorTestCase {
	return map[string]includeSelectorTestCase{
		"selector keeps matching " + kind + " instance": {include: []string{match}, want: 1},
		"selector keeps matching " + kind + " prefix":   {include: []string{prefix + ":" + match}, want: 1},
		"selector keeps matching instance prefix":       {include: []string{"instance:" + match}, want: 1},
		"selector keeps all " + kind + " instances":     {include: []string{"*"}, want: 2},
		"selector can exclude all " + kind + " instances": {
			include: []string{missing},
			want:    0,
		},
	}
}

func instanceSelectorCases(kind, match, missing string) map[string]includeSelectorTestCase {
	return map[string]includeSelectorTestCase{
		"selector keeps matching " + kind + " instance": {include: []string{match}, want: 1},
		"selector keeps matching instance prefix":       {include: []string{"instance:" + match}, want: 1},
		"selector keeps all " + kind + " instances":     {include: []string{"*"}, want: 2},
		"selector can exclude all " + kind + " instances": {
			include: []string{missing},
			want:    0,
		},
	}
}
