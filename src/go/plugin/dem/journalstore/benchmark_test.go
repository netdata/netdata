// SPDX-License-Identifier: GPL-3.0-or-later

package journalstore

import (
	"context"
	"strconv"
	"testing"

	"github.com/netdata/systemd-journal-sdk/go/journal"
)

// Append includes the shared envelope and SDK publication, but not per-row
// fsync. Distinct receipt clocks and repeated context exercise normal postings.
// Cost follows fields per event; timing is a local trend, never a CI limit.
func BenchmarkHistoryAppend(b *testing.B) {
	ctx := context.Background()
	store, err := Open(ctx, "")
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	fields := []journal.Field{
		journal.StringField("DEM_KIND", "rum"), journal.StringField("DEM_OBSERVED_US", "0"),
		journal.StringField("DEM_REVISION", "1"), journal.StringField("DEM_SITE", "shop"),
		journal.StringField("DEM_SESSION_ID", "session"), journal.StringField("DEM_TYPE", "error"),
		journal.StringField("DEM_PAGE", "/checkout"), journal.StringField("DEM_BROWSER", "Firefox"),
		journal.StringField("DEM_COUNTRY", "GR"), journal.StringField("DEM_DEVICE", "desktop"),
		journal.StringField("DEM_VERSION", "release-1"), journal.StringField("DEM_USER_ID", "user-id"),
		journal.StringField("DEM_FINGERPRINT", "fingerprint"), journal.StringField("DEM_ERROR_TYPE", "TypeError"),
		journal.StringField("DEM_MESSAGE", "request failed"), journal.StringField("DEM_SAMPLE_STACK", "app.js:10"),
		journal.StringField("MESSAGE", "request failed"),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		fields[1].Value = strconv.AppendInt(fields[1].Value[:0], 1_700_000_000_000_000+int64(i)*1_000_000, 10)
		if _, err := store.Append(ctx, fields); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
}
