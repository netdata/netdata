// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"strings"
	"testing"
)

// The unextended baseline stream measures parser cost independently of optional
// reporting volume. It is also buildable against the frozen contract-2 parser.
func BenchmarkRuntimeCoreParsing(b *testing.B) {
	input := strings.Repeat(`{"type":"event","event":{"kind":"step","at_ms":1,"title":"normal step","status":"passed"}}`+"\n", 30) + `{"type":"result","result":{"status":"success","tests":{"declared":1,"passed":1,"failed":0,"timed_out":0,"skipped":0,"expected_failure":0,"not_run":0},"capture_state":"disabled"}}` + "\n"
	b.ReportAllocs()
	for b.Loop() {
		s := newSink(synthetic.Journey, false, newRedactor(nil), false, false)
		if err := s.consume(strings.NewReader(input)); err != nil {
			b.Fatal(err)
		}
		_ = s.snapshot()
	}
}
