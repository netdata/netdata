// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"cmp"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/assert"
)

// TestFrameEnvelopes pins which frames fail the stream and the error each one
// records; frame errors reach the run's stored error text.
func TestFrameEnvelopes(t *testing.T) {
	const (
		step   = `{"type":"event","event":{"kind":"step"}}`
		result = `{"type":"result","result":{"status":"error","capture_state":"disabled"}}`
	)
	tests := map[string]struct {
		kind         synthetic.Kind
		lines        []string
		unterminated bool // the last frame ends with the data, as when a read returns data with EOF
		err          string
	}{
		"event":                        {lines: []string{step}},
		"event with null result":       {lines: []string{`{"type":"event","event":{"kind":"step"},"result":null}`}, err: "invalid runner event envelope"},
		"event with extension":         {lines: []string{`{"type":"event","event":{"kind":"step"},"extension":{}}`}, err: "invalid runner event envelope"},
		"event without body":           {lines: []string{`{"type":"event"}`}, err: "missing runner core body"},
		"event with null body":         {lines: []string{`{"type":"event","event":null}`}, err: "missing runner core body"},
		"event with unknown member":    {lines: []string{`{"type":"event","event":{"kind":"step","extra":1}}`}, err: `invalid runner core body: json: unknown field "extra"`},
		"result with event":            {lines: []string{`{"type":"result","event":{"kind":"step"},"result":{}}`}, err: "invalid runner result envelope"},
		"result with extension_init":   {lines: []string{`{"type":"result","extension_init":{}}`}, err: "invalid runner result envelope"},
		"extension_init with data":     {lines: []string{`{"type":"extension_init","extension":{}}`}, err: "mixed runner extension envelope"},
		"extension_init with result":   {lines: []string{`{"type":"extension_init","result":{}}`}, err: "mixed runner extension envelope"},
		"extension with init":          {lines: []string{`{"type":"extension","extension_init":{}}`}, err: "mixed runner extension envelope"},
		"extension with event":         {lines: []string{`{"type":"extension","event":{}}`}, err: "mixed runner extension envelope"},
		"extension_init with null":     {lines: []string{`{"type":"extension_init","extension_init":null}`}},
		"extension with null":          {lines: []string{`{"type":"extension","extension":null}`}},
		"lighthouse ignores extension": {kind: synthetic.Lighthouse, lines: []string{`{"type":"extension","extension":{"feature":"x"}}`}},
		"unknown type":                 {lines: []string{`{"type":"future"}`}, err: "unsupported runner frame type"},
		"unknown type with event":      {lines: []string{`{"type":"future","event":{"kind":"step"}}`}, err: "unsupported runner frame type"},
		"unknown member":               {lines: []string{`{"type":"future","future":{}}`}, err: `invalid runner protocol frame: json: unknown field "future"`},
		"not JSON":                     {lines: []string{`not JSON`}, err: "invalid runner protocol frame: invalid character 'o' in literal null (expecting 'u')"},
		"two values":                   {lines: []string{step + ` {}`}, err: "multiple values in runner protocol frame"},
		"event after result":           {lines: []string{result, step}, err: "runner sent a frame after its terminal result"},
		"unknown type after result":    {lines: []string{result, `{"type":"future"}`}, err: "runner sent a frame after its terminal result"},
		"frame at limit":               {lines: []string{strings.Repeat(" ", FrameMaxBytes-len(step)) + step}},
		"frame one byte over":          {lines: []string{strings.Repeat(" ", FrameMaxBytes+1-len(step)) + step}, err: "read runner protocol: bufio.Scanner: token too long"},
		"unterminated frame over":      {lines: []string{strings.Repeat(" ", FrameMaxBytes+1-len(step)) + step}, unterminated: true, err: "runner frame exceeds 4 MiB transport limit"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			kind := cmp.Or(tc.kind, synthetic.Journey)
			s := newSink(synthetic.Request{Kind: kind, Capture: false}, newRedactor(nil))
			var input io.Reader = strings.NewReader(strings.Join(tc.lines, "\n") + "\n")
			if tc.unterminated {
				input = iotest.DataErrReader(strings.NewReader(strings.Join(tc.lines, "\n")))
			}
			err := s.consume(input)
			if tc.err == "" {
				assert.NoError(t, err)
				assert.NoError(t, s.snapshot().err)
				return
			}
			if assert.Error(t, err) {
				assert.Equal(t, tc.err, err.Error())
				assert.Equal(t, err, s.snapshot().err)
			}
		})
	}
}
