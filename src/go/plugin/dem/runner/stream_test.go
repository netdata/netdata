// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/assert"
)

type fragmentReader struct{ chunks []string }

func (r *fragmentReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[0])
	r.chunks[0] = r.chunks[0][n:]
	if len(r.chunks[0]) == 0 {
		r.chunks = r.chunks[1:]
	}
	return n, nil
}
func diagnosticText(events []synthetic.Event) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteString(e.Message)
	}
	return b.String()
}
func TestBootstrapStderrRedactsAcrossReadBoundaries(t *testing.T) {
	s := newSink(synthetic.Journey, false, newRedactor(map[string]string{"DEM_SECRET_TOKEN": "fixture-only-secret-0123456789", "DEM_SECRET_UTF": "秘密🔐value"}))
	chunks := []string{"before fixture-only-secret-", "0123456789\n", "token=generic-", "credential-tail\n", "-----BE", "GIN PRIVATE KEY-----\n", "private-material\n", "-----END PRIVATE KEY-----\n", "safe after\n"}
	for _, b := range []byte("秘密🔐value\n") {
		chunks = append(chunks, string([]byte{b}))
	}
	chunks = append(chunks, "unfinished fixture-only-secret-")
	s.stderr(&fragmentReader{chunks: chunks})
	recovered := diagnosticText(s.snapshot().events)
	for _, secret := range []string{"fixture-only-secret-", "秘密", "generic-", "credential-tail", "private-material"} {
		assert.NotContains(t, recovered, secret)
	}
	assert.Contains(t, recovered, "safe after")
	assert.Contains(t, recovered, "[REDACTED]")
}

func TestDiagnosticStreamsKeepBoundariesAndEvidence(t *testing.T) {
	secret := "fixture-only-secret-0123456789"
	r := newRedactor(map[string]string{"DEM_SECRET_TOKEN": secret})
	s := newSink(synthetic.Journey, false, r)
	var wire strings.Builder
	for _, event := range []synthetic.Event{
		{Kind: "stdout", Phase: "worker", Message: "before fixture-only-secret-"},
		{Kind: "step", Phase: "expect", Title: "assertion evidence", Status: "failed"},
		{Kind: "stdout", Phase: "cli", Message: "other stream\n"},
		{Kind: "stdout", Phase: "worker", Message: "0123456789\n"},
		{Kind: "stderr", Phase: "worker", Message: "Bearer\n"},
		{Kind: "stderr", Phase: "worker", Message: "split-generic-credential\n"},
		{Kind: "stdout", Phase: "worker", Message: "last fixture-only-secret-"},
	} {
		raw, _ := json.Marshal(frame{Type: "event", Event: &event})
		wire.Write(raw)
		wire.WriteByte('\n')
	}
	assert.NoError(t, s.consume(strings.NewReader(wire.String())))
	got := s.snapshot()
	recovered := diagnosticText(got.events)
	assert.NotContains(t, recovered, "fixture-only-secret-")
	assert.NotContains(t, recovered, "split-generic-credential")
	assert.Contains(t, recovered, "other stream")
	assert.Contains(t, recovered, "last [REDACTED]")
	assert.True(t, len(got.events) > 0 && got.events[0].Kind == "step", "structured evidence remains incremental while a stream is buffered")
}
func TestDiagnosticOverflowAndRetentionCannotExposeTails(t *testing.T) {
	secret := strings.Repeat("private-fixture-", 300)
	s := newSink(synthetic.Journey, false, newRedactor(map[string]string{"DEM_SECRET_TOKEN": secret}))
	chunks := []string{secret[:2001], secret[2001:] + "\n", strings.Repeat("safe ", TextLimit), "token=hidden-tail\n", "-----BEGIN PRIVATE KEY-----\n", strings.Repeat("private-material", 1000), "\n-----END -----\nstill-private\n-----END PRIVATE KEY-----\n", strings.Repeat("safe line\n", EventLimit+5), "unfinished " + secret[:80]}
	s.stderr(&fragmentReader{chunks: chunks})
	got := s.snapshot()
	recovered := diagnosticText(got.events)
	assert.Len(t, got.events, EventLimit)
	assert.Greater(t, got.dropped, 0)
	for _, part := range []string{"private-fixture-", "hidden-tail", "private-material", "still-private"} {
		assert.NotContains(t, recovered, part)
	}
	assert.Contains(t, recovered, "[REDACTED]")
	assert.Contains(t, recovered, "diagnostic line exceeded")
	for _, event := range got.events {
		assert.LessOrEqual(t, utf8.RuneCountInString(event.Message), TextLimit)
	}
}
func TestExactStreamOverlapUnicodeAndShortValues(t *testing.T) {
	values := map[string]string{"DEM_SECRET_A": "abc", "DEM_SECRET_B": "abcdef", "DEM_SECRET_U": "秘密🔐value", "DEM_SECRET_N": "first\nsecond", "DEM_SECRET_S": "!"}
	input := "ordinary 界 abc abcdef 秘密🔐value first\nsecond ! tail abcde"
	for width := 1; width <= len(input); width++ {
		s := newSink(synthetic.Journey, false, newRedactor(values))
		var chunks []string
		for left := input; len(left) > 0; {
			n := min(width, len(left))
			chunks = append(chunks, left[:n])
			left = left[n:]
		}
		s.stderr(&fragmentReader{chunks: chunks})
		got := diagnosticText(s.snapshot().events)
		assert.Equal(t, "ordinary 界 [REDACTED] [REDACTED] [REDACTED] [REDACTED] [REDACTED] tail [REDACTED]\n", got, "width %d", width)
		assert.True(t, utf8.ValidString(got))
	}
}

func TestStructuredPEMRemainsRedactedWhenUpstreamClipsEnd(t *testing.T) {
	message := "context -----BEGIN PRIVATE KEY-----\n" + strings.Repeat("private-key-material", 200) + "\n-----END PRIVATE KEY-----"
	assert.Equal(t, "context [REDACTED]", newRedactor(nil).text(clip(message)))
}

func BenchmarkDiagnosticLongSecretCarry(b *testing.B) {
	for _, size := range []int{64 << 10, 1 << 20, 4 << 20} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			secret := strings.Repeat("s", size)
			input := strings.Repeat("x", size) + "\n"
			r := newRedactor(map[string]string{"DEM_SECRET_TOKEN": secret})
			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			b.ResetTimer()
			for b.Loop() {
				d := newDiagnosticStream(r, func(string) {})
				d.write(input)
				d.end()
			}
		})
	}
}
