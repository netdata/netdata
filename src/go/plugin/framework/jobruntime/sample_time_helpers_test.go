// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// sampleTimes maps each chart sample in the output, keyed "<host>/<type.id>", to
// the time its END carries ("<seconds> <microseconds> 1"), "" for a plain END, or
// "BEGIN ..." when its BEGIN carries an interval.
func sampleTimes(wire string) map[string]string {
	out := make(map[string]string)
	host, chart := "", ""
	for line := range strings.SplitSeq(wire, "\n") {
		switch {
		case strings.HasPrefix(line, "HOST '"):
			host = strings.TrimSuffix(strings.TrimPrefix(line, "HOST '"), "'")
		case strings.HasPrefix(line, "BEGIN '"):
			id, extra, _ := strings.Cut(strings.TrimPrefix(line, "BEGIN '"), "'")
			chart = host + "/" + id
			if extra != "" {
				out[chart] = "BEGIN" + extra
				chart = ""
			}
		case line == "END" || strings.HasPrefix(line, "END "):
			if chart != "" {
				out[chart] = strings.TrimPrefix(strings.TrimPrefix(line, "END"), " ")
				chart = ""
			}
		}
	}
	return out
}

// endAt is the END time of a sample collected at t (whole microseconds).
func endAt(t time.Time) string {
	us := t.UnixMicro()
	return fmt.Sprintf("%d %d 1", us/1e6, us%1e6)
}

var timedEND = regexp.MustCompile(`(?m)^END \d+ \d+ 1$`)

// plainENDs replaces the sample times of timed END lines with a plain END, for
// tests that check wire structure rather than timing.
func plainENDs(wire string) string {
	return timedEND.ReplaceAllString(wire, "END")
}
