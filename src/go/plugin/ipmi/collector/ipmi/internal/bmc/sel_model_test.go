// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"context"
	"encoding/binary"
	"errors"
	"maps"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/bougou/go-ipmi/pkg/command/storage"
	"github.com/bougou/go-ipmi/pkg/types"
)

// TestReader_SEL_RandomLogChanges runs the Reader through random sequences of
// BMC log changes (appends, rollover with free or slot-style reused record
// IDs, records Get SEL Info cannot show, clears, single deletions, rewrites,
// and bursts, clears, deletions and deletions whose ID a new record takes at
// once while a read is in progress) and failures
// (Get SEL Info and Get SEL Entry errors, cancellations, cancelled
// reservations, multi-collection reads). After a known baseline, each eligible
// sample is checked against independently received wire identities, with mixed
// event classes from the real-log fixture. Seeds alternate a known baseline and
// changes during the initial load. Once changes stop, the cache must
// equal the fake BMC's log. Ledger size is checked during changes too.
func TestReader_SEL_RandomLogChanges(t *testing.T) {
	templates := append(readReplaySEL(t), syntheticCriticalEvents(t, 1000, 0x60000000)...)
	for seed := range uint64(500) {
		if !runSELModel(t, seed+1, 150, templates) {
			return
		}
	}
}

// receivingTransport counts the records the Reader received.
type receivingTransport struct {
	*fakeBMC
	received map[[selRecordSize]byte]int
}

func (o *receivingTransport) Exchange(ctx context.Context, req types.Request, res types.Response) error {
	err := o.fakeBMC.Exchange(ctx, req, res)
	if q, ok := res.(*storage.GetSELEntryResponse); ok && err == nil && ctx.Err() == nil && len(q.Data) == selRecordSize {
		o.received[[selRecordSize]byte(q.Data)]++
	}
	return err
}

// selModel logs records into a fake BMC log with a fixed capacity. On a full
// log a slot-style BMC gives a new record the ID of the record it overwrites;
// otherwise record IDs cycle through a small ID space, so rollover reuses the
// IDs of overwritten records later.
type selModel struct {
	fake      *fakeBMC
	capacity  int
	slot      bool
	idSpace   uint16
	lastID    uint16
	ts        uint32
	templates [][selRecordSize]byte
}

// log logs a record. A hidden record leaves the addition time as it was, like
// a record logged in the same second as the previous one.
func (m *selModel) log(hidden bool) {
	m.ts++
	var rec [selRecordSize]byte
	if m.slot && len(m.fake.sel.records) >= m.capacity {
		rec = m.fake.sel.overwrite(m.ts, hidden)
	} else {
		present := make(map[uint16]bool)
		for _, held := range m.fake.sel.records {
			present[binary16(held)] = true
		}
		for {
			m.lastID = m.lastID%m.idSpace + 1
			if !present[m.lastID] {
				break
			}
		}
		rec = tempEvent(m.lastID, m.ts)
		m.fake.sel.records = append(m.fake.sel.records, rec)
		if len(m.fake.sel.records) > m.capacity {
			m.fake.sel.records = m.fake.sel.records[1:]
		}
		if !hidden {
			m.fake.sel.additionTime++
		}
		m.fake.sel.reservation = 0
	}
	m.fake.sel.records[len(m.fake.sel.records)-1] = m.record(binary16(rec))
}

// record stamps the current timestamp and ID onto a mixed-class fixture.
func (m *selModel) record(id uint16) [selRecordSize]byte {
	raw := m.templates[int(m.ts)%len(m.templates)]
	binary.LittleEndian.PutUint16(raw[:], id)
	binary.LittleEndian.PutUint32(raw[3:], m.ts)
	return raw
}

// clear empties the log; the BMC then logs that the log was cleared.
func (m *selModel) clear() {
	m.fake.sel.clear()
	m.log(false)
}

// deleteOne deletes one record, which changes the erase time, and reports
// its record ID.
func (m *selModel) deleteOne(rng *rand.Rand) (uint16, bool) {
	n := len(m.fake.sel.records)
	if n == 0 {
		return 0, false
	}
	i := rng.IntN(n)
	id := binary16(m.fake.sel.records[i])
	m.fake.sel.records = slices.Delete(slices.Clone(m.fake.sel.records), i, i+1)
	m.fake.sel.eraseTime++
	m.fake.sel.reservation = 0
	return id, true
}

// replaceOne deletes one record and logs a new record under its ID.
func (m *selModel) replaceOne(rng *rand.Rand) {
	if id, ok := m.deleteOne(rng); ok {
		m.ts++
		m.fake.sel.add(m.record(id))
	}
}

func binary16(rec [selRecordSize]byte) uint16 { return uint16(rec[0]) | uint16(rec[1])<<8 }

// runSELModel runs one seed and reports whether its checks passed.
func runSELModel(t *testing.T, seed uint64, steps int, templates [][selRecordSize]byte) bool {
	t.Helper()
	rng := rand.New(rand.NewPCG(seed, seed*7+1))
	fake := newFakeBMC()
	capacity := []int{3, 6, 10}[rng.IntN(3)]
	m := &selModel{
		fake:      fake,
		capacity:  capacity,
		slot:      rng.IntN(2) == 0,
		idSpace:   []uint16{uint16(capacity + 1), uint16(2 * capacity), 40}[rng.IntN(3)],
		ts:        0x60000000,
		templates: slices.Clone(templates),
	}
	rng.Shuffle(len(m.templates), func(i, j int) {
		m.templates[i], m.templates[j] = m.templates[j], m.templates[i]
	})
	fake.sel.requireReservation = rng.IntN(2) == 0
	for range 1 + rng.IntN(6) {
		m.log(false)
	}
	history := slices.Clone(fake.sel.records)

	receiving := &receivingTransport{fakeBMC: fake, received: make(map[[selRecordSize]byte]int)}
	r := newTestReader(fake)
	r.newTransport = func() (transport, error) { return receiving, nil }
	r.cfg.UpdateEvery = 4 * time.Second
	now := testNow
	r.now = func() time.Time { return now }
	var step, eventsDuringRead int
	var entryCost time.Duration
	fake.sel.onEntry = func() {
		now = now.Add(entryCost)
		if eventsDuringRead == 0 || rng.IntN(3) != 0 {
			return
		}
		eventsDuringRead--
		switch rng.IntN(7) {
		case 0:
			for range 1 + rng.IntN(capacity+1) {
				m.log(false)
			}
		case 1:
			m.clear()
		case 2:
			m.deleteOne(rng)
		case 3:
			m.replaceOne(rng)
		default:
			m.log(rng.IntN(4) == 0)
		}
	}

	// Half the seeds start from a known baseline; the others retain initial-
	// load interleavings. No SEL entry is read after baseline verification in
	// that collection, so its received records are still history.
	started := seed%2 == 0
	if started {
		baseline, err := r.Collect(t.Context(), true)
		if err != nil || baseline.SEL == nil || !baseline.SEL.Loaded {
			t.Fatalf("seed %d: initial baseline: %v", seed, err)
		}
	}
	accounted := make(map[[selRecordSize]byte]bool)
	for _, raw := range history {
		accounted[raw] = true
	}
	expected := make(map[newRecordKey]int)
	collect := func(failures bool) *SEL {
		entryCost = []time.Duration{0, 300 * time.Millisecond}[rng.IntN(2)] // all, or 4 records per collection
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		if failures {
			switch rng.IntN(6) {
			case 0:
				fake.selErr = errors.New("transport failed")
			case 1, 2:
				failAt, cancelAt := 1+rng.IntN(5), rng.IntN(2) == 0
				n := 0
				fake.onExchange = func(req types.Request) error {
					if _, ok := req.(*storage.GetSELEntryRequest); ok {
						if n++; n == failAt {
							if cancelAt {
								cancel()
								return nil
							}
							return errors.New("transport failed")
						}
					}
					return nil
				}
			}
		}
		snapshot, err := r.Collect(ctx, true)
		fake.selErr, fake.onExchange = nil, nil
		if !started {
			for raw := range receiving.received {
				accounted[raw] = true
			}
			started = r.sel.seen != nil
		} else {
			addNewCounts(expected, newlyReceivedSEL(receiving, accounted))
		}
		if len(r.sel.seen) > int(m.idSpace) {
			t.Errorf("seed %d step %d: ledger has %d entries for %d IDs", seed, step, len(r.sel.seen), m.idSpace)
		}
		if err != nil || snapshot.SEL == nil {
			return nil
		}
		sel := snapshot.SEL
		if sel.NewKnown {
			if !maps.Equal(expected, countNew(sel.New)) {
				t.Errorf("seed %d step %d: counts %v, want observed %v", seed, step, countNew(sel.New), expected)
			}
			clear(expected)
		} else if started && !sel.ReadFailed {
			t.Errorf("seed %d step %d: accounting unavailable during a successful rescan", seed, step)
		}
		return sel
	}

	for step = 0; step < steps; step++ {
		switch rng.IntN(10) {
		case 0, 1, 2, 3:
			hidden := rng.IntN(4) == 0
			for range 1 + rng.IntN(3) {
				m.log(hidden)
			}
		case 4:
			m.clear()
		case 5:
			m.deleteOne(rng)
		case 6:
			if n := len(fake.sel.records); n > 0 {
				m.ts++
				rec := m.record(binary16(fake.sel.records[n-1]))
				fake.sel.records = append(slices.Clone(fake.sel.records[:n-1]), rec)
				fake.sel.additionTime++
				fake.sel.reservation = 0
			}
		case 7:
			eventsDuringRead = 1 + rng.IntN(2)
		}
		collect(true)
	}
	eventsDuringRead = 0
	// Records Get SEL Info could not show stay unread until a change it shows.
	m.log(false)

	var sel *SEL
	for quiet := 0; quiet < 3; {
		if step++; step > steps+60 {
			t.Errorf("seed %d: the Reader did not settle", seed)
			return false
		}
		if sel = collect(false); sel != nil && sel.Loaded && sel.NewKnown && sel.New == (SELCounts{}) {
			quiet++
		} else {
			quiet = 0
		}
	}

	if !slices.Equal(fake.sel.records, recordsRaw(sel)) {
		t.Errorf("seed %d: cache %x, BMC log %x", seed, recordsRaw(sel), fake.sel.records)
	}
	if len(r.sel.seen) != len(r.sel.records) {
		t.Errorf("seed %d: seen holds %d contents for %d cached records", seed, len(r.sel.seen), len(r.sel.records))
	}
	for _, rec := range sel.Records {
		if r.sel.seen[rec.ID] != rec.Raw {
			t.Errorf("seed %d: settled occupant differs at ID %d", seed, rec.ID)
		}
	}
	return !t.Failed()
}
