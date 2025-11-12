package output

import "testing"

func TestParsePerfdata(t *testing.T) {
	raw := []byte("OK - all good | 'time'=123ms;200;500;0;1000 'load1'=0.12;1.0;2.0\nLong output line\nAnother | ignored")
	parsed := Parse(raw)
	if parsed.StatusLine != "OK - all good" {
		t.Fatalf("unexpected status line: %s", parsed.StatusLine)
	}
	if parsed.LongOutput != "Long output line\nAnother" {
		t.Fatalf("unexpected long output: %s", parsed.LongOutput)
	}
	if len(parsed.Perfdata) != 2 {
		t.Fatalf("expected 2 perfdata entries, got %d", len(parsed.Perfdata))
	}
	if parsed.Perfdata[0].Label != "time" || parsed.Perfdata[0].Unit != "ms" || parsed.Perfdata[0].Value != 123 {
		t.Fatalf("unexpected first perfdatum: %+v", parsed.Perfdata[0])
	}
	if parsed.Perfdata[1].Label != "load1" || parsed.Perfdata[1].Value != 0.12 {
		t.Fatalf("unexpected second perfdatum: %+v", parsed.Perfdata[1])
	}
}
