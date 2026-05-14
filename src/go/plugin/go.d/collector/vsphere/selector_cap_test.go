// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

type includeCapTestCase struct {
	include []string
	max     int
	want    int
}

func prefixedSelectorCapCases(kind, prefix, match, missing string) map[string]includeCapTestCase {
	return map[string]includeCapTestCase{
		"selector keeps matching " + kind + " instance": {include: []string{match}, max: 10, want: 1},
		"selector keeps matching " + kind + " prefix":   {include: []string{prefix + ":" + match}, max: 10, want: 1},
		"selector keeps matching instance prefix":       {include: []string{"instance:" + match}, max: 10, want: 1},
		"cap limits emitted " + kind + " instances":     {include: []string{"*"}, max: 1, want: 1},
		"selector can exclude all " + kind + " instances": {
			include: []string{missing},
			max:     10,
			want:    0,
		},
	}
}

func instanceSelectorCapCases(kind, match, missing string) map[string]includeCapTestCase {
	return map[string]includeCapTestCase{
		"selector keeps matching " + kind + " instance": {include: []string{match}, max: 10, want: 1},
		"selector keeps matching instance prefix":       {include: []string{"instance:" + match}, max: 10, want: 1},
		"cap limits emitted " + kind + " instances":     {include: []string{"*"}, max: 1, want: 1},
		"selector can exclude all " + kind + " instances": {
			include: []string{missing},
			max:     10,
			want:    0,
		},
	}
}
