// SPDX-License-Identifier: GPL-3.0-or-later
package vnodeguid

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolve(t *testing.T) {
	const guid = "11111111-1111-4111-8111-111111111111"
	for name, tc := range map[string]struct {
		args map[string]string
		want string
		err  string
	}{
		"all":                        {args: nil},
		"empty GUID retains all":     {args: map[string]string{"vnode_guid": ""}},
		"name":                       {args: map[string]string{"vnode": "storefront"}, want: guid},
		"GUID without configuration": {args: map[string]string{"vnode_guid": guid}, want: guid},
		"unknown":                    {args: map[string]string{"vnode": "removed"}, err: "unknown configured vnode"},
		"empty name":                 {args: map[string]string{"vnode": ""}, err: "configured vnode name"},
		"both":                       {args: map[string]string{"vnode": "storefront", "vnode_guid": guid}, err: "mutually exclusive"},
		"both empty":                 {args: map[string]string{"vnode": "", "vnode_guid": ""}, err: "mutually exclusive"},
		"invalid GUID":               {args: map[string]string{"vnode_guid": "invalid"}, err: "canonical UUID"},
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			got, err := Resolve(tc.args, func(name string) (string, bool) {
				calls++
				return guid, name == "storefront"
			})
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
			if name == "name" || name == "unknown" {
				require.Equal(t, 1, calls)
			} else {
				require.Zero(t, calls)
			}
		})
	}
	_, err := Resolve(map[string]string{"vnode": "storefront"}, nil)
	require.ErrorContains(t, err, "lookup is unavailable")
}
