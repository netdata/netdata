// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readReplaySEL returns the records of testdata/sel-replay.hex, a real BMC's
// System Event Log in log order.
func readReplaySEL(t *testing.T) [][selRecordSize]byte {
	t.Helper()
	f, err := os.Open("testdata/sel-replay.hex")
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	var records [][selRecordSize]byte
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		raw, err := hex.DecodeString(line)
		require.NoError(t, err)
		require.Len(t, raw, selRecordSize)
		records = append(records, [selRecordSize]byte(raw))
	}
	require.NoError(t, scanner.Err())
	return records
}

type newRecordKey struct {
	threshold bool
	state     string
}

func countRecords(records []SELRecord) map[newRecordKey]int {
	counts := make(map[newRecordKey]int)
	for _, rec := range records {
		counts[newRecordKey{rec.Threshold, rec.State}]++
	}
	return counts
}

func countNew(c SELCounts) map[newRecordKey]int {
	all := map[newRecordKey]uint64{
		{true, StateNominal}: c.Threshold.Nominal, {true, StateWarning}: c.Threshold.Warning,
		{true, StateCritical}: c.Threshold.Critical, {true, StateUnknown}: c.Threshold.Unknown,
		{false, StateNominal}: c.Discrete.Nominal, {false, StateWarning}: c.Discrete.Warning,
		{false, StateCritical}: c.Discrete.Critical, {false, StateUnknown}: c.Discrete.Unknown,
	}
	counts := make(map[newRecordKey]int)
	for key, n := range all {
		if n != 0 {
			counts[key] = int(n)
		}
	}
	return counts
}

func countRaw(records [][selRecordSize]byte) map[newRecordKey]int {
	decoded := make([]SELRecord, 0, len(records))
	for _, raw := range records {
		decoded = append(decoded, decodeSELRecord(raw, nil))
	}
	return countRecords(decoded)
}

// syntheticCriticalEvents are discrete events that the FreeIPMI default
// policy rates critical when asserted, logged after the replayed records.
// They are not from the real log.
func syntheticCriticalEvents(t *testing.T, firstID uint16, timestamp uint32) [][selRecordSize]byte {
	t.Helper()
	events := []struct {
		name                          string
		eventType, sensorType, offset int
	}{
		{"IPMI_Processor_IERR", 0x6f, 0x07, 0},
		{"IPMI_Power_Supply_Power_Supply_Failure_Detected", 0x6f, 0x08, 1},
		{"IPMI_Power_Unit_AC_Lost", 0x6f, 0x09, 4},
		{"IPMI_Memory_Uncorrectable_Memory_Error", 0x6f, 0x0c, 1},
		{"IPMI_Critical_Interrupt_Bus_Uncorrectable_Error", 0x6f, 0x13, 8},
		{"IPMI_Temperature_Limit_Exceeded", 0x05, 0x01, 1},
	}
	assertion := make(map[string]string)
	for _, row := range readSELStatesFixture(t) {
		assertion[fmt.Sprintf("%s/%s/%s", row[0], row[1], row[2])] = row[4] + " " + row[3]
	}
	var records [][selRecordSize]byte
	for i, e := range events {
		key := fmt.Sprintf("%d/%d/%d", e.eventType, e.sensorType, e.offset)
		require.Equal(t, StateCritical+" "+e.name, assertion[key], "FreeIPMI default for %s", key)
		records = append(records, selEvent(firstID+uint16(i), timestamp+uint32(i), 0x0020,
			byte(e.sensorType), byte(100+i), byte(e.eventType), byte(e.offset), 0xff, 0xff))
	}
	return records
}

// TestReader_SEL_Replay replays a real BMC's log. Loaded at start, none of it
// is new; logged after the first read, its critical records are all
// threshold events, so only the synthetic critical discrete events, appended
// last, count as critical discrete records.
func TestReader_SEL_Replay(t *testing.T) {
	replayed := readReplaySEL(t)
	require.Len(t, replayed, 672)
	last := decodeSELRecord(replayed[len(replayed)-1], nil)
	synthetic := syntheticCriticalEvents(t, last.ID+1, last.Timestamp+60)
	// Counts by event type and state, as the ipmi-sel Function showed this log on
	// its host. They pin this decoder's classification of real records; the
	// FreeIPMI fixture test checks the states themselves.
	wantReal := map[newRecordKey]int{
		{true, StateNominal}:  136,
		{true, StateWarning}:  220,
		{true, StateCritical}: 78,
		{false, StateNominal}: 68,
		{false, StateWarning}: 144,
		{false, StateUnknown}: 26,
	}

	t.Run("history at start", func(t *testing.T) {
		fake := newFakeBMC()
		fake.sel.records = replayed
		r := newSELTestReader(fake, 10*time.Second, 8*time.Millisecond)
		var sel *SEL
		for sel == nil || !sel.Loaded {
			sel = collectSEL(t, r)
			assert.Equal(t, sel.Loaded, sel.NewKnown)
			assert.Empty(t, sel.New)
		}
		assert.Len(t, sel.Records, len(replayed))

		fake.sel.add(synthetic...)
		sel = collectSEL(t, r)
		assert.Equal(t, map[newRecordKey]int{{false, StateCritical}: len(synthetic)}, countNew(sel.New))
	})

	t.Run("logged after the first read", func(t *testing.T) {
		fake := newFakeBMC()
		r := newSELTestReader(fake, 10*time.Second, 8*time.Millisecond)
		require.True(t, collectSEL(t, r).NewKnown)

		got := make(map[newRecordKey]int)
		for batch := range slices.Chunk(replayed, 100) {
			fake.sel.add(batch...)
			sel := collectSEL(t, r)
			require.True(t, sel.NewKnown)
			addNewCounts(got, countNew(sel.New))
		}
		assert.Equal(t, wantReal, got)
		assert.Zero(t, got[newRecordKey{false, StateCritical}], "no critical discrete record")

		fake.sel.add(synthetic...)
		sel := collectSEL(t, r)
		assert.Equal(t, map[newRecordKey]int{{false, StateCritical}: len(synthetic)}, countNew(sel.New))
	})
}
