// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-015 — receiver discards in-flight data on child disconnect.
//
// When a child closes its socket right after its last write, the parent's
// stream thread handles the poll HUP event BEFORE the pending READ
// (stream-receiver.c stream_receive_process_poll_events: the
// ND_POLL_ERROR|ND_POLL_HUP branch removes the receiver without draining),
// discarding data TCP had already delivered — from a few tail rows up to
// the entire burst, depending on timing. Real-world victims: the final
// samples of every child that restarts or crashes, and a child that
// completes replication and rotates to another parent, which can lose the
// whole replicated window.
//
// Both cases push a burst far larger than the receiver can drain before the
// FIN registers (30k rows ≈ 1.7MB in ONE write() syscall — the receiver
// reads ~15KB per poll cycle), then disconnect immediately. RED until the
// receiver drains readable data before honoring HUP.
package corpus

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

const case015Points = 30000

func case015Settled(t *testing.T, hostname string, ch fixture.Chart) bool {
	t.Helper()
	ret, err := td.WaitRetention(hostname, ch.Context, ch.FirstT(), ch.LastT(), 10*time.Second)
	if err != nil {
		t.Logf("data lost: %v", err)
		return false
	}
	t.Logf("no loss: retention [%d,%d]", ret.FirstEntry, ret.LastEntry)
	return true
}

func TestCase015LiveDisconnectDiscard(t *testing.T) {
	ch := fixture.FullPalette("fixture.c015live", "fixture.c015live", fixture.T0, case015Points)

	conn, err := stream.Connect(td.Addr, daemon.StreamKey,
		stream.HostInfo{Hostname: "c015-live", MachineGUID: guid(15)}, stream.CapsLive)
	if err != nil {
		t.Fatal(err)
	}
	ch.Define(conn)
	ch.PushLive(conn)
	// deliberate immediate close: the bug's trigger
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	expectAgentStatus(t, "CASE-015/live-disconnect-discard", case015Settled(t, "c015-live", ch))
}

// TestCase015MidDialogueDisconnect declares retention and disconnects
// BEFORE serving the parent's replication request: the parent parses
// CHART_DEFINITION_END from drained data after the child is gone and its
// queued REPLAY_CHART reply fails to send — the historically fragile
// teardown interaction (stream-receiver.c parser/opcode protection). The
// daemon must survive and keep serving. Green on fixed and unfixed builds;
// it guards crash-safety, not data completeness.
func TestCase015MidDialogueDisconnect(t *testing.T) {
	ch := fixture.FullPalette("fixture.c015mid", "fixture.c015mid", fixture.T0, 60)

	conn, err := stream.Connect(td.Addr, daemon.StreamKey,
		stream.HostInfo{Hostname: "c015-mid", MachineGUID: guid(17)}, stream.CapsReplication)
	if err != nil {
		t.Fatal(err)
	}
	ch.Define(conn)
	conn.ChartDefinitionEnd(ch.FirstT()-1, ch.LastT(), ch.LastT())
	// deliberate close BEFORE the replication dialogue: the parent's
	// REPLAY_CHART reply will hit a dead socket
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)

	// the daemon must still be alive and able to serve a fresh child
	fresh := fixture.FullPalette("fixture.c015after", "fixture.c015after", fixture.T0, 60)
	pushLiveBurst(t, "c015-after", guid(18), fresh)
	settleAndVerify(t, "c015-after", fresh)
}

// TestCase015DisconnectSoak hammers the connect/burst/immediate-close cycle
// to shake races out of the teardown path. Liveness guard only — data
// completeness is asserted by the discard cases above.
func TestCase015DisconnectSoak(t *testing.T) {
	const cycles = 30
	for i := 0; i < cycles; i++ {
		host := fmt.Sprintf("c015-soak-%02d", i)
		ch := fixture.FullPalette(fmt.Sprintf("fixture.c015s%02d", i), fmt.Sprintf("fixture.c015s%02d", i), fixture.T0, 500)
		conn, err := stream.Connect(td.Addr, daemon.StreamKey,
			stream.HostInfo{Hostname: host, MachineGUID: guid(100 + i)}, stream.CapsLive)
		if err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}
		ch.Define(conn)
		ch.PushLive(conn)
		if err := conn.Close(); err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}
	}
	time.Sleep(3 * time.Second)

	// the daemon must have survived all teardowns and still serve
	fresh := fixture.FullPalette("fixture.c015soakend", "fixture.c015soakend", fixture.T0, 60)
	pushLiveBurst(t, "c015-soak-end", guid(99), fresh)
	settleAndVerify(t, "c015-soak-end", fresh)
}

func TestCase015ReplicationDisconnectDiscard(t *testing.T) {
	ch := fixture.FullPalette("fixture.c015repl", "fixture.c015repl", fixture.T0, case015Points)

	conn, err := stream.Connect(td.Addr, daemon.StreamKey,
		stream.HostInfo{Hostname: "c015-repl", MachineGUID: guid(16)}, stream.CapsReplication)
	if err != nil {
		t.Fatal(err)
	}
	firstT, lastT := ch.FirstT()-1, ch.LastT()
	ch.Define(conn)
	conn.ChartDefinitionEnd(firstT, lastT, lastT)

	charts := map[string]struct{ FirstT, LastT int64 }{
		ch.ID: {FirstT: firstT, LastT: lastT},
	}
	served, err := conn.ServeReplication(charts, lastT, func(chart string, after, before int64) []stream.ReplayRow {
		return ch.ReplayWindow(after, before)
	}, 30*time.Second)
	if err != nil {
		t.Fatalf("replication dialogue: %v (served %v)", err, served)
	}
	// deliberate immediate close right after the final REND: the bug's trigger
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	expectAgentStatus(t, "CASE-015/replication-disconnect-discard", case015Settled(t, "c015-repl", ch))
}
