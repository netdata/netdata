// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Find(t *testing.T) {
	test := map[string]struct {
		sysObjOId   string
		wanProfiles int
	}{
		"mikrotik": {
			sysObjOId:   "1.3.6.1.4.1.14988.1",
			wanProfiles: 2,
		},
		"no match": {
			sysObjOId:   "0.1.2.3",
			wanProfiles: 0,
		},
	}

	for name, test := range test {
		t.Run(name, func(t *testing.T) {
			profiles := Find(test.sysObjOId)

			require.Len(t, profiles, test.wanProfiles)
		})
	}
}
