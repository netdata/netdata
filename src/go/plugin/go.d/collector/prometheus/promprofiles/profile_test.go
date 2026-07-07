// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecodeProfile_validation checks header and template validation through the
// real decode path. A user profile (isStock=false) validates its template at
// load, so every rule — header (app format) and template (no chart, undeclared
// dimension) — surfaces from decodeProfile.
func TestDecodeProfile_validation(t *testing.T) {
	tests := map[string]struct {
		yaml    string
		wantErr bool
	}{
		"valid": {
			yaml: `match: "a_*"
template:
  family: fam
  metrics:
    - a_total
  charts:
    - title: Title
      context: ctx
      units: count
      dimensions:
        - selector: a_total
          name: d0
`,
		},
		"valid with app": {
			yaml: `match: "a_*"
app: my_app
template:
  family: fam
  metrics:
    - a_total
  charts:
    - title: Title
      context: ctx
      units: count
      dimensions:
        - selector: a_total
          name: d0
`,
		},
		"invalid app format": {
			yaml: `match: "a_*"
app: "Bad App!"
template:
  family: fam
  metrics:
    - a_total
  charts:
    - title: Title
      context: ctx
      units: count
      dimensions:
        - selector: a_total
          name: d0
`,
			wantErr: true,
		},
		"template with no chart": {
			yaml: `match: "a_*"
template:
  family: fam
`,
			wantErr: true,
		},
		"dimension metric not declared in metrics": {
			yaml: `match: "a_*"
template:
  family: fam
  metrics:
    - declared_total
  charts:
    - title: T
      context: ctx
      units: count
      dimensions:
        - selector: undeclared_total
          name: d
`,
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := decodeProfile([]byte(tc.yaml), "test", false)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
