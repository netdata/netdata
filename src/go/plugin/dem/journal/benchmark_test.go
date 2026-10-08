// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"context"
	"strconv"
	"testing"

	sdk "github.com/netdata/systemd-journal-sdk/go/journal"
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
	fields := []sdk.Field{
		sdk.StringField("DEM_KIND", "rum"), sdk.StringField("DEM_OBSERVED_US", "0"),
		sdk.StringField("DEM_REVISION", "1"), sdk.StringField("DEM_SITE", "shop"),
		sdk.StringField("DEM_SESSION_ID", "session"), sdk.StringField("DEM_TYPE", "error"),
		sdk.StringField("DEM_PAGE", "/checkout"), sdk.StringField("DEM_BROWSER", "Firefox"),
		sdk.StringField("DEM_COUNTRY", "GR"), sdk.StringField("DEM_DEVICE", "desktop"),
		sdk.StringField("DEM_VERSION", "release-1"), sdk.StringField("DEM_USER_ID", "user-id"),
		sdk.StringField("DEM_FINGERPRINT", "fingerprint"), sdk.StringField("DEM_ERROR_TYPE", "TypeError"),
		sdk.StringField("DEM_MESSAGE", "request failed"), sdk.StringField("DEM_SAMPLE_STACK", "app.js:10"),
		sdk.StringField("MESSAGE", "request failed"),
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
