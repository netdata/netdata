// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-016 — a child that first connects shortly before a GRACEFUL parent
// restart is entirely forgotten: host metadata persistence is asynchronous
// (a scan cycle every METADATA_HOST_CHECK_INTERVAL=5s,
// sqlite_metadata.c) and the metasync shutdown path flushes pending alerts
// and SQL statements but never runs a final host scan — the host record
// never reaches sqlite, and on the next boot the host 404s while its
// dbengine files sit orphaned on disk.
//
// Real-world: a parent restarting (e.g. updating) within seconds of a new
// child's first connection forgets that child and its streamed history.
// Children that reconnect with local retention re-register and re-replicate
// (self-healing); ephemeral or no-retention children lose the data
// permanently — the same healing asymmetry as CASE-015.
//
// The bug is a RACE against the 5s scan phase: if a scan tick happens to
// fire between the fresh host's connection and the shutdown, the host is
// persisted and that attempt does not reproduce (seen under full-suite
// load). The case therefore retries with a brand-new fresh host up to
// three times — reproducing once proves the bug; three consecutive
// survivals demand the manifest flip (that is the fixed behavior, where
// the shutdown flush persists the host regardless of scan phase).
package corpus

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/fixture"
)

func TestCase016FreshHostForgottenOnRestart(t *testing.T) {
	aged := fixture.Series("fixture.c016aged", "fixture.c016aged", fixture.T0, 60, 1, modVal, notAnom)
	pushLiveBurst(t, "c016-aged", guid(40), aged)
	settleAndVerify(t, "c016-aged", aged)

	reproduced := false
	for attempt := 1; attempt <= 3 && !reproduced; attempt++ {
		context := fmt.Sprintf("fixture.c016fresh%d", attempt)
		hostname := fmt.Sprintf("c016-fresh-%d", attempt)
		fresh := fixture.Series(context, context, fixture.T0, 60, 1, modVal, notAnom)

		// let a metadata scan cycle pass, so everything already connected
		// (including the aged control) is persisted and the fresh host
		// below is the only pending one
		time.Sleep(8 * time.Second)

		pushLiveBurst(t, hostname, guid(65+attempt), fresh)
		settleAndVerify(t, hostname, fresh)

		if err := td.Restart(); err != nil {
			t.Fatal(err)
		}

		// control: the aged host must survive every graceful restart
		if _, err := td.WaitRetention("c016-aged", aged.Context, aged.FirstT(), aged.LastT(), 10*time.Second); err != nil {
			t.Errorf("control failed: aged host lost across restart: %v", err)
		}

		// the bug: the fresh host is forgotten
		if _, err := td.WaitRetention(hostname, fresh.Context, fresh.FirstT(), fresh.LastT(), 10*time.Second); err != nil {
			t.Logf("attempt %d: fresh host lost: %v", attempt, err)
			reproduced = true
		} else {
			t.Logf("attempt %d: fresh host survived (a scan tick fired inside the race window) — retrying", attempt)
		}
	}

	expectAgentStatus(t, "CASE-016/fresh-host-forgotten-on-restart", !reproduced)
}
