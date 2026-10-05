// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func errBeacon(session, page, browser string, errs ...beacon.Error) *beacon.Beacon {
	b := mk(t0, session, page)
	b.Browser = browser
	b.Errors = errs
	return b
}
func TestErrorsRankCurrentWindowAndFoldComplement(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := range 15 {
		b := mk(*now, "s", "/")
		b.Events = nil
		b.Errors = []beacon.Error{{Fingerprint: fmt.Sprint(i)}}
		a.Ingest(b)
	}
	groups := a.Snapshot().ErrorGroups
	require.Len(t, groups, 11)
	var count uint64
	for _, g := range groups {
		count += g.Count
	}
	assert.EqualValues(t, 15, count)
	assert.True(t, groups[10].Other)
	assert.EqualValues(t, 5, groups[10].Count)
	for i, fp := range []string{"0", "1", "10", "11", "12", "13", "14", "2", "3", "4"} {
		assert.Equal(t, fp, groups[i].Fingerprint)
		assert.EqualValues(t, 1, groups[i].Count)
	}
	*now = now.Add(time.Minute + time.Second)
	b := mk(*now, "s", "/")
	b.Errors = []beacon.Error{{Fingerprint: "current"}}
	a.Ingest(b)
	groups = a.Snapshot().ErrorGroups
	require.Len(t, groups, 1)
	assert.Equal(t, "current", groups[0].Fingerprint)
	assert.EqualValues(t, 1, groups[0].Count)
}
func TestTruncateStackKeepsValidUTF8(t *testing.T) {
	s := truncateStack(strings.Repeat("界", errMaxStackBytes))
	assert.True(t, utf8.ValidString(s))
	assert.LessOrEqual(t, len(s), errMaxStackBytes)
}
