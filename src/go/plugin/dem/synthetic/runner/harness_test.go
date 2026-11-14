// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

func pointerResult(r reporterResult) *reporterResult { return &r }

// frame builds baseline fixture output independently of the live wire decoder.
type frame struct {
	Type   string           `json:"type"`
	Event  *synthetic.Event `json:"event,omitempty"`
	Result *reporterResult  `json:"result,omitempty"`
}

func emit(writer io.Writer, value any) {
	raw, _ := json.Marshal(value)
	_, _ = fmt.Fprintln(writer, string(raw))
}

func success() reporterResult {
	return reporterResult{
		Status: synthetic.Success,
		Tests: &synthetic.TestCounts{
			Declared: 1,
			Passed:   1,
		},
		CaptureState: "disabled",
	}
}

func featureLine(feature, data string) string {
	return `{"type":"extension","extension":{"feature":"` + feature + `","data":` + data + `}}`
}
