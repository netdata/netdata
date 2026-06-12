// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/stretchr/testify/assert"
)

// chartGroup builds a minimal valid charttpl group with an explicit metrics:
// visibility list (declared as a real v2 template would), matching the
// dimension selectors.
func chartGroup(family, context string, selectors ...string) charttpl.Group {
	dims := make([]charttpl.Dimension, len(selectors))
	for i, sel := range selectors {
		dims[i] = charttpl.Dimension{Selector: sel, Name: fmt.Sprintf("dim%d", i)}
	}
	return charttpl.Group{
		Family:  family,
		Metrics: append([]string(nil), selectors...),
		Charts:  []charttpl.Chart{{Title: "Title", Context: context, Units: "count", Dimensions: dims}},
	}
}

func TestProfile_validate(t *testing.T) {
	tests := map[string]struct {
		profile Profile
		wantErr bool
	}{
		"valid": {
			profile: Profile{Match: "a_*", Template: chartGroup("fam", "ctx", "a_total", "a_count")},
		},
		"valid with app": {
			profile: Profile{Match: "a_*", App: "my_app", Template: chartGroup("fam", "ctx", "a_total")},
		},
		"invalid app format": {
			profile: Profile{Match: "a_*", App: "Bad App!", Template: chartGroup("fam", "ctx", "a_total")},
			wantErr: true,
		},
		"empty match": {
			profile: Profile{Match: "", Template: chartGroup("fam", "ctx", "a_total")},
			wantErr: true,
		},
		"template with no chart": {
			profile: Profile{Match: "a_*", Template: charttpl.Group{Family: "fam"}},
			wantErr: true,
		},
		"dimension metric not declared in metrics": {
			profile: Profile{Match: "a_*", Template: charttpl.Group{
				Family:  "fam",
				Metrics: []string{"declared_total"},
				Charts: []charttpl.Chart{{Title: "T", Context: "ctx", Units: "count",
					Dimensions: []charttpl.Dimension{{Selector: "undeclared_total", Name: "d"}}}},
			}},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p := tc.profile
			err := p.validate("test")
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
