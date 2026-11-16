// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMethodParamValuesFromArguments(t *testing.T) {
	for name, test := range map[string]struct {
		args    []string
		payload string
		want    []string
	}{
		"repeated values preserve order": {
			args: []string{"service:events,audit", "service=maintenance", "service:,security,"},
			want: []string{"events", "audit", "maintenance", "security"},
		},
		"empty repetition preserves earlier values": {
			args: []string{"service:events", "service:,"},
			want: []string{"events"},
		},
		"arguments override payload selections": {
			args:    []string{"service:events", "service:audit"},
			payload: `{"selections":{"service":["maintenance"]}}`,
			want:    []string{"events", "audit"},
		},
		"empty arguments allow payload selections": {
			args:    []string{"info", "service:", "service:,"},
			payload: `{"selections":{"service":["maintenance"]}}`,
			want:    []string{"maintenance"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := methodParamValues(
				parseMethodArguments(test.args),
				parseMethodPayload([]byte(test.payload)),
				"service",
			)
			assert.Equal(t, test.want, got)
		})
	}
}
