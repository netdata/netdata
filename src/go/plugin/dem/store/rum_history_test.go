// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRumHistoryTablesCreatedIdempotent(t *testing.T) {
	s, dir := newStore(t)
	// Reopening must preserve previously stored sessions.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(context.Background(), fmt.Sprintf("%s/test.db", dir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s2.Close() }()
	if _, _, err := s2.WriteRumHistoryBatch(context.Background(), RumHistoryBatch{
		Sessions: []RumSessionRecord{{Site: "s", SessionID: "sess1", StartedAt: 1, LastAt: 1}},
	}); err != nil {
		t.Fatalf("rum_sessions table must exist after reopen: %v", err)
	}
	rows, err := s2.QueryRumSessions(context.Background(), "s", 0, time.Now().Unix()+1000, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("session must survive reopen: rows=%v err=%v", rows, err)
	}
}

func TestRumSessionUpsert(t *testing.T) {
	s, _ := newStore(t)
	rec := RumSessionRecord{
		Site:      "s",
		SessionID: "sess1",
		StartedAt: 100,
		LastAt:    100,
		Pageviews: 1,
		Errors:    0,
		Browser:   "Chrome",
		Device:    "desktop",
		Country:   "GR",
		City:      "Athens",
		Version:   "1.0",
		EntryPage: "/a",
		LastPage:  "/a",
	}
	if _, _, err := s.WriteRumHistoryBatch(context.Background(), RumHistoryBatch{
		Sessions: []RumSessionRecord{rec},
	}); err != nil {
		t.Fatal(err)
	}
	// Progress the same session: started_at/entry_page must stick, the rest updates.
	rec2 := rec
	rec2.LastAt = 200
	rec2.Pageviews = 2 // two new views, added to the first contribution
	rec2.Errors = 1
	rec2.LastPage = "/b"
	rec2.StartedAt = 999 // must be ignored on conflict
	rec2.EntryPage = "/ignored"
	if _, _, err := s.WriteRumHistoryBatch(context.Background(), RumHistoryBatch{
		Sessions: []RumSessionRecord{rec2},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryRumSessions(context.Background(), "s", 0, 1000, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 session row (upsert, not insert), got %d", len(got))
	}
	g := got[0]
	if g.StartedAt != 100 || g.EntryPage != "/a" {
		t.Fatalf("started_at/entry_page must not change on conflict: %+v", g)
	}
	if g.LastAt != 200 || g.Pageviews != 3 || g.Errors != 1 || g.LastPage != "/b" {
		t.Fatalf("progressed fields must update: %+v", g)
	}
	if g.City != "Athens" {
		t.Fatalf("city must persist: %+v", g)
	}
}

func TestRumSessionEventInsertAndQuery(t *testing.T) {
	s, _ := newStore(t)
	evs := []RumSessionEventRecord{
		{Site: "s", SessionID: "sess1", TSUnixUS: 3_000_000, Type: "pageview", Page: "/b", Text: ""},
		{Site: "s", SessionID: "sess1", TSUnixUS: 1_000_000, Type: "pageview", Page: "/a", Text: ""},
		{Site: "s", SessionID: "sess1", TSUnixUS: 2_000_000, Type: "error", Page: "/a", Text: "boom"},
	}
	if _, _, err := s.WriteRumHistoryBatch(context.Background(), RumHistoryBatch{
		Events: evs,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryRumSessionEvents(context.Background(), "s", "sess1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d", len(got))
	}
	// oldest first
	if got[0].TSUnixUS != 1_000_000 || got[1].TSUnixUS != 2_000_000 || got[2].TSUnixUS != 3_000_000 {
		t.Fatalf("events must be ordered oldest-first: %+v", got)
	}
}

// TestRumSessionEventsUnfilteredSite pins site=="" as "any site" (matches
// the in-memory Aggregator.SessionEvents lookup rum-session-events falls
// back to), not a literal empty-string site filter.
func TestRumSessionEventsUnfilteredSite(t *testing.T) {
	s, _ := newStore(t)
	if _, _, err := s.WriteRumHistoryBatch(context.Background(), RumHistoryBatch{
		Events: []RumSessionEventRecord{
			{Site: "shop", SessionID: "sess1", TSUnixUS: 1, Type: "pageview", Page: "/a"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryRumSessionEvents(context.Background(), "", "sess1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected the session to be found across sites, got %+v", got)
	}
	if none, err := s.QueryRumSessionEvents(context.Background(), "other-site", "sess1"); err != nil || len(none) != 0 {
		t.Fatalf("a specific, non-matching site filter must still exclude it: %+v err=%v", none, err)
	}
}

func TestRumSessionEventCap(t *testing.T) {
	s, _ := newStore(t)
	var evs []RumSessionEventRecord
	for i := 0; i < sessionEventCap+50; i++ {
		evs = append(
			evs,
			RumSessionEventRecord{
				Site:      "s",
				SessionID: "sess1",
				TSUnixUS:  int64(i),
				Type:      "pageview",
				Page:      "/a",
			},
		)
	}
	written, dropped, err := s.WriteRumHistoryBatch(context.Background(), RumHistoryBatch{
		Events: evs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if written["s"] != sessionEventCap {
		t.Fatalf("written = %d, want %d", written["s"], sessionEventCap)
	}
	if dropped["s"] != 50 {
		t.Fatalf("dropped = %d, want 50", dropped["s"])
	}
	got, err := s.QueryRumSessionEvents(context.Background(), "s", "sess1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != sessionEventCap {
		t.Fatalf("stored events = %d, want cap %d", len(got), sessionEventCap)
	}

	// A second session must not be affected by the first's cap.
	if _, _, err := s.WriteRumHistoryBatch(context.Background(), RumHistoryBatch{
		Events: []RumSessionEventRecord{
			{Site: "s", SessionID: "sess2", TSUnixUS: 1, Type: "pageview", Page: "/a"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.QueryRumSessionEvents(context.Background(), "s", "sess2")
	if len(got2) != 1 {
		t.Fatalf("second session must not be capped by the first's count: %d", len(got2))
	}
}

func TestRumErrorGroupUpsertKeepsFirstSeenAndSample(t *testing.T) {
	s, _ := newStore(t)
	g1 := RumErrorGroupRecord{
		Site:        "s",
		Fingerprint: "fp1",
		Type:        "TypeError",
		Message:     "boom",
		SampleStack: "at f()",
		FirstSeen:   100,
		LastSeen:    100,
	}
	if _, _, err := s.WriteRumHistoryBatch(context.Background(), RumHistoryBatch{
		ErrGroups: []RumErrorGroupRecord{g1},
	}); err != nil {
		t.Fatal(err)
	}
	g2 := g1
	g2.FirstSeen = 999     // must be ignored on conflict
	g2.Message = "changed" // must be ignored on conflict (fixed at first sight, matches in-memory semantics)
	g2.SampleStack = "other"
	g2.LastSeen = 200
	if _, _, err := s.WriteRumHistoryBatch(context.Background(), RumHistoryBatch{
		ErrGroups: []RumErrorGroupRecord{g2},
	}); err != nil {
		t.Fatal(err)
	}
	occs, err := s.QueryRumErrors(context.Background(), "s", 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	// No occurrences recorded, so nothing should show from QueryRumErrors
	// (it aggregates occurrences); verify group persistence directly instead.
	if len(occs) != 0 {
		t.Fatalf("no occurrences inserted yet, expected 0 aggregated rows, got %+v", occs)
	}
	var msg, stack string
	var first, last int64
	row := s.db.QueryRow(
		`SELECT message, sample_stack, first_seen, last_seen FROM rum_error_groups WHERE site=? AND fingerprint=?`,
		"s",
		"fp1",
	)
	if err := row.Scan(&msg, &stack, &first, &last); err != nil {
		t.Fatal(err)
	}
	if msg != "boom" || stack != "at f()" {
		t.Fatalf("message/sample_stack must stick from first insert: %q %q", msg, stack)
	}
	if first != 100 || last != 200 {
		t.Fatalf("first_seen must stick, last_seen must update: first=%d last=%d", first, last)
	}
}

func TestQueryRumErrorsAggregatesOverRange(t *testing.T) {
	s, _ := newStore(t)
	if _, _, err := s.WriteRumHistoryBatch(context.Background(), RumHistoryBatch{
		ErrGroups: []RumErrorGroupRecord{
			{Site: "s", Fingerprint: "fp1", Type: "TypeError", Message: "boom", SampleStack: "stk", FirstSeen: 100, LastSeen: 100},
		},
	}); err != nil {
		t.Fatal(err)
	}
	occs := []RumErrorOccurrenceRecord{
		{Site: "s", Fingerprint: "fp1", TS: 100, SessionID: "sess1", Page: "/a", Browser: "Chrome"},
		{Site: "s", Fingerprint: "fp1", TS: 150, SessionID: "sess1", Page: "/a", Browser: "Chrome"},
		{Site: "s", Fingerprint: "fp1", TS: 200, SessionID: "sess2", Page: "/b", Browser: "Firefox"},
		{Site: "s", Fingerprint: "fp1", TS: 9999, SessionID: "sess3", Page: "/c", Browser: "Safari"}, // out of range
	}
	if _, _, err := s.WriteRumHistoryBatch(context.Background(), RumHistoryBatch{
		ErrOccs: occs,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryRumErrors(context.Background(), "s", 50, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 aggregated group, got %+v", got)
	}
	r := got[0]
	if r.CountWindow != 3 {
		t.Fatalf("count_window = %d, want 3 (range excludes the 9999 occurrence)", r.CountWindow)
	}
	if r.SessionsAffected != 2 {
		t.Fatalf("sessions_affected = %d, want 2", r.SessionsAffected)
	}
	if r.FirstSeen != 100 || r.LastSeen != 200 {
		t.Fatalf("first/last seen in range = %d/%d, want 100/200", r.FirstSeen, r.LastSeen)
	}
	if r.TopPage != "/a" {
		t.Fatalf("top_page = %q, want /a (2 occurrences vs 1)", r.TopPage)
	}
	if len(r.Browsers) != 2 || r.Browsers[0] != "Chrome" {
		t.Fatalf("browsers = %+v, want [Chrome Firefox]", r.Browsers)
	}
	if r.Message != "boom" || r.Type != "TypeError" || r.SampleStack != "stk" {
		t.Fatalf("group fields not joined: %+v", r)
	}
}

func TestQueryRumSessionsOverlapSemantics(t *testing.T) {
	s, _ := newStore(t)
	batch := RumHistoryBatch{Sessions: []RumSessionRecord{
		{Site: "s", SessionID: "before", StartedAt: 0, LastAt: 50},          // ends before range
		{Site: "s", SessionID: "overlap-start", StartedAt: 50, LastAt: 150}, // starts before, ends inside
		{Site: "s", SessionID: "inside", StartedAt: 120, LastAt: 180},       // fully inside
		{Site: "s", SessionID: "overlap-end", StartedAt: 190, LastAt: 300},  // starts inside, ends after
		{Site: "s", SessionID: "after", StartedAt: 500, LastAt: 600},        // starts after range
	}}
	if _, _, err := s.WriteRumHistoryBatch(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryRumSessions(context.Background(), "s", 100, 200, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"overlap-start": true, "inside": true, "overlap-end": true}
	if len(got) != len(want) {
		t.Fatalf("got %d sessions, want %d: %+v", len(got), len(want), got)
	}
	for _, g := range got {
		if !want[g.SessionID] {
			t.Fatalf("unexpected session in overlap result: %s", g.SessionID)
		}
	}
	// newest last_at first
	if got[0].SessionID != "overlap-end" {
		t.Fatalf("expected newest-first ordering, got %+v", got)
	}
}

func TestQueryRumSessionsCap(t *testing.T) {
	s, _ := newStore(t)
	var recs []RumSessionRecord
	for i := 0; i < 2500; i++ {
		recs = append(
			recs,
			RumSessionRecord{
				Site:      "s",
				SessionID: fmt.Sprintf("sess%04d", i),
				StartedAt: int64(i),
				LastAt:    int64(i),
			},
		)
	}
	if _, _, err := s.WriteRumHistoryBatch(context.Background(), RumHistoryBatch{
		Sessions: recs,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryRumSessions(context.Background(), "s", 0, 3000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2000 {
		t.Fatalf("expected the 2000 cap, got %d", len(got))
	}
}

func TestRumHistoryRetentionByAge(t *testing.T) {
	s, _ := newStore(t)
	now := time.Now().Unix()
	old := now - 40*86400
	fresh := now - 1*86400

	if _, _, err := s.WriteRumHistoryBatch(context.Background(), RumHistoryBatch{
		Sessions:  []RumSessionRecord{{Site: "s", SessionID: "old", StartedAt: old, LastAt: old}, {Site: "s", SessionID: "fresh", StartedAt: fresh, LastAt: fresh}},
		Events:    []RumSessionEventRecord{{Site: "s", SessionID: "old", TSUnixUS: old * 1_000_000, Type: "pageview"}, {Site: "s", SessionID: "fresh", TSUnixUS: fresh * 1_000_000, Type: "pageview"}},
		ErrGroups: []RumErrorGroupRecord{{Site: "s", Fingerprint: "old", FirstSeen: old, LastSeen: old}, {Site: "s", Fingerprint: "fresh", FirstSeen: fresh, LastSeen: fresh}},
		ErrOccs:   []RumErrorOccurrenceRecord{{Site: "s", Fingerprint: "old", TS: old}, {Site: "s", Fingerprint: "fresh", TS: fresh}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.EnforceRumHistoryRetention(context.Background(), 30, 0); err != nil {
		t.Fatal(err)
	}
	sessions, _ := s.QueryRumSessions(context.Background(), "s", 0, now+1, 10)
	if len(sessions) != 1 || sessions[0].SessionID != "fresh" {
		t.Fatalf("age retention must keep only the fresh session: %+v", sessions)
	}
	events, _ := s.QueryRumSessionEvents(context.Background(), "s", "old")
	if len(events) != 0 {
		t.Fatalf("old session's events must be gone: %+v", events)
	}
	errs, _ := s.QueryRumErrors(context.Background(), "s", 0, now+1)
	if len(errs) != 1 || errs[0].Fingerprint != "fresh" {
		t.Fatalf("age retention must keep only the fresh error group's occurrences: %+v", errs)
	}
}

// TestRumHistoryRetentionByAgePrunesOrphanSession covers a session whose
// last_at was kept fresh by beacons that produced no event (e.g. repeat
// vitals within the pageview dedup window): the events cutoff alone
// removes its one stored event while last_at stays newer than the
// cutoff, so the session row would otherwise survive with nothing left
// pointing to it.
func TestRumHistoryRetentionByAgePrunesOrphanSession(t *testing.T) {
	s, _ := newStore(t)
	now := time.Now().Unix()
	oldEvent := now - 40*86400
	freshLastAt := now - 1*86400

	if _, _, err := s.WriteRumHistoryBatch(context.Background(), RumHistoryBatch{
		Sessions: []RumSessionRecord{{Site: "s", SessionID: "sess1", StartedAt: oldEvent, LastAt: freshLastAt}},
		Events:   []RumSessionEventRecord{{Site: "s", SessionID: "sess1", TSUnixUS: oldEvent * 1_000_000, Type: "pageview"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.EnforceRumHistoryRetention(context.Background(), 30, 0); err != nil {
		t.Fatal(err)
	}
	events, err := s.QueryRumSessionEvents(context.Background(), "s", "sess1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("the old event must be gone: %+v", events)
	}
	sessions, err := s.QueryRumSessions(context.Background(), "s", 0, now+1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("session with zero remaining events must be pruned even though last_at was still fresh: %+v", sessions)
	}
}

func TestRumHistoryRetentionBySizeEvictsOldestDayFirst(t *testing.T) {
	s, _ := newStore(t)
	now := time.Now().Unix()
	// Three distinct days of error occurrences, each with a sizable page value.
	bigPage := strings.Repeat("x", 500)
	var occs []RumErrorOccurrenceRecord
	for day := 0; day < 3; day++ {
		ts := now - int64(2-day)*86400 // day0 oldest .. day2 newest
		for i := 0; i < 5; i++ {
			occs = append(
				occs,
				RumErrorOccurrenceRecord{
					Site:        "s",
					Fingerprint: fmt.Sprintf("fp%d", day),
					TS:          ts,
					Page:        bigPage,
					Browser:     "Chrome",
				},
			)
		}
	}
	if _, _, err := s.WriteRumHistoryBatch(context.Background(), RumHistoryBatch{
		ErrOccs: occs,
	}); err != nil {
		t.Fatal(err)
	}
	before, err := s.rumHistoryBytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Cap tight enough to force evicting the oldest day only.
	capBytes := before - 1
	if err := s.EnforceRumHistoryRetention(context.Background(), 0, capBytes); err != nil {
		t.Fatal(err)
	}
	after, err := s.rumHistoryBytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if after > capBytes {
		t.Fatalf("size after retention (%d) must be <= cap (%d)", after, capBytes)
	}
	remaining, err := s.QueryRumErrors(context.Background(), "s", 0, now+1)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range remaining {
		if r.Fingerprint == "fp0" {
			t.Fatalf("oldest day's fingerprint fp0 must be evicted first: %+v", remaining)
		}
	}
}

func TestRumHistoryRetentionSizeCapPrunesOrphanParents(t *testing.T) {
	s, _ := newStore(t)
	now := time.Now().Unix()
	oldDay := now - 2*86400
	bigText := strings.Repeat("y", 1000)

	if _, _, err := s.WriteRumHistoryBatch(context.Background(), RumHistoryBatch{
		Sessions: []RumSessionRecord{{Site: "s", SessionID: "sess1", StartedAt: oldDay, LastAt: oldDay}},
		Events:   []RumSessionEventRecord{{Site: "s", SessionID: "sess1", TSUnixUS: oldDay * 1_000_000, Type: "pageview", Text: bigText}},
	}); err != nil {
		t.Fatal(err)
	}
	before, err := s.rumHistoryBytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnforceRumHistoryRetention(context.Background(), 0, before-1); err != nil {
		t.Fatal(err)
	}
	sessions, err := s.QueryRumSessions(context.Background(), "s", 0, now+1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("session with all events evicted must be pruned too: %+v", sessions)
	}
}

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, dir
}
