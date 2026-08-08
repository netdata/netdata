// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package rasdaemon

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// summary is the parsed result of `ras-mc-ctl --summary`.
//
// Every count is cumulative since the rasdaemon database was created: the underlying tables are
// append-only and rasdaemon never prunes them, so these values are monotonic and are written to
// metrix as counters.
type summary struct {
	// memory holds one entry per (DIMM label, error type). rasdaemon reports a physical DIMM
	// once per csrow, so the same label appears on several input lines; parse sums them.
	memory []dimmEvents
	// aer, mce and memoryFailure are keyed by the error type/message ras-mc-ctl groups by.
	aer           []typedEvents
	mce           []typedEvents
	memoryFailure []typedEvents
	// classes holds aggregate totals for the error classes this collector does not break out
	// individually (extlog, devlink, disk, cxl, arm, signal).
	classes map[string]int64
}

func (s *summary) isEmpty() bool {
	return len(s.memory) == 0 && len(s.aer) == 0 && len(s.mce) == 0 &&
		len(s.memoryFailure) == 0 && len(s.classes) == 0
}

type dimmEvents struct {
	dimm    string
	errType string
	count   int64
}

type typedEvents struct {
	errType string
	count   int64
}

// section identifies which `ras-mc-ctl --summary` block the parser is currently inside.
//
// Section tracking is REQUIRED, not a convenience: the per-line grammars are mutually ambiguous.
// "\t2 Corrected errors: Receiver Error" (AER: count first) and "\tRecovered errors: 1"
// (memory failure: count last) both match a naive "<words> errors: <words>" pattern, and
// "\tsdd has 1 errors" (disk) collides with the ARM CPU form. Parsing a line without knowing
// its section silently mis-attributes counts.
type section int

const (
	sectionNone section = iota
	sectionMemory
	sectionAER
	sectionMCE
	sectionMemoryFailure
	sectionExtlog
	sectionDevlink
	sectionDisk
	sectionARM
	sectionCXL
	sectionSignal
)

// Aggregate class names used for sections this collector reports as a single total.
const (
	classExtlog  = "extlog"
	classDevlink = "devlink"
	classDisk    = "disk"
	classARM     = "arm"
	classCXL     = "cxl"
	classSignal  = "signal"
)

var (
	// "\tCorrected on DIMM Label(s): 'DDR4_A1' location: 0:0:0:0 errors: 2"
	reMemory = regexp.MustCompile(`^(\S+) on DIMM Label\(s\): '(.*)' location: (\S+) errors: (\d+)$`)
	// "\t2 Corrected errors: Receiver Error"  (count first, trailing message)
	reCountTypeMsg = regexp.MustCompile(`^(\d+) (.+?) errors: (.*)$`)
	// "\tRecovered errors: 1" / "\tmem0 errors: 5" / "\t2 errors: 14"  (count last)
	reTypeCount = regexp.MustCompile(`^(.*) errors: (\d+)$`)
	// "\t1 Uncorrected error errors" / "\t3 mem-uc fatal errors"  (count first, no message)
	reCountType = regexp.MustCompile(`^(\d+) (.+) errors$`)
	// "\tsdd has 1 errors" / "\t0000:41:00.0 has 1 errors" / "\tCPU(mpidr=0x0) has 2 errors"
	reDevCount = regexp.MustCompile(`^(.+) has (\d+) errors$`)
)

// sectionHeaders maps a normalized populated-section header to its section.
//
// Only the populated headers matter. The "No <x> errors." lines carry no counts, so they need no
// entry: they simply reset the parser to sectionNone like any other unrecognized header.
//
// Keys are matched after whitespace normalization because ras-mc-ctl emits
// "CXL  generic events summary:" with a double space (upstream typo).
var sectionHeaders = map[string]section{
	"Memory controller events summary:":     sectionMemory,
	"PCIe AER events summary:":              sectionAER,
	"MCE records summary:":                  sectionMCE,
	"Memory failure events summary:":        sectionMemoryFailure,
	"Extlog records summary:":               sectionExtlog,
	"Devlink records summary:":              sectionDevlink,
	"Disk errors summary:":                  sectionDisk,
	"ARM processor events summary:":         sectionARM,
	"SIGNAL events summary:":                sectionSignal,
	"CXL AER uncorrectable events summary:": sectionCXL,
	"CXL AER correctable events summary:":   sectionCXL,
	"CXL overflow events summary:":          sectionCXL,
	"CXL poison events summary:":            sectionCXL,
	"CXL generic events summary:":           sectionCXL,
	"CXL general media events summary:":     sectionCXL,
	"CXL DRAM events summary:":              sectionCXL,
	"CXL memory module events summary:":     sectionCXL,
}

var wsRun = regexp.MustCompile(`\s+`)

func normalizeHeader(s string) string { return wsRun.ReplaceAllString(strings.TrimSpace(s), " ") }

// parseSummary parses `ras-mc-ctl --summary` output.
//
// An empty input is an error rather than "no errors": ras-mc-ctl exits non-zero with empty stdout
// when a table it was compiled for is missing from the database (for example rasdaemon built
// without sqlite3, or never run with --record). Reporting zeros there would silently mask a broken
// RAS pipeline, which is the very failure this collector exists to surface.
func parseSummary(data []byte) (*summary, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("empty output (ras-mc-ctl may have failed; check that the rasdaemon database exists and is readable)")
	}

	sm := &summary{classes: make(map[string]int64)}
	// Memory counts are accumulated by (label, err_type) because one physical DIMM is reported
	// once per csrow and therefore spans multiple lines.
	memAcc := make(map[dimmKey]int64)
	var memOrder []dimmKey

	cur := sectionNone
	sawSection := false

	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for sc.Scan() {
		raw := sc.Text()
		if strings.TrimSpace(raw) == "" {
			continue
		}

		// Detail lines are tab-indented; anything else is a section header.
		if !strings.HasPrefix(raw, "\t") && !strings.HasPrefix(raw, " ") {
			if s, ok := sectionHeaders[normalizeHeader(raw)]; ok {
				cur, sawSection = s, true
			} else {
				// "No Memory errors." and friends, or an unknown future header.
				cur = sectionNone
				if strings.HasPrefix(raw, "No ") {
					sawSection = true
				}
			}
			continue
		}

		line := strings.TrimSpace(raw)
		if cur == sectionNone {
			continue
		}
		if err := sm.parseLine(cur, line, memAcc, &memOrder); err != nil {
			return nil, err
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading ras-mc-ctl output: %w", err)
	}

	// Output that contained no recognizable section at all is not a healthy machine; it means the
	// command produced something we do not understand (wrong binary, truncated run, changed format).
	if !sawSection {
		return nil, errors.New("no recognizable sections in ras-mc-ctl output")
	}

	for _, k := range memOrder {
		sm.memory = append(sm.memory, dimmEvents{dimm: k.dimm, errType: k.errType, count: memAcc[k]})
	}

	return sm, nil
}

type dimmKey struct {
	dimm    string
	errType string
}

func (s *summary) parseLine(sec section, line string, memAcc map[dimmKey]int64, memOrder *[]dimmKey) error {
	switch sec {
	case sectionMemory:
		m := reMemory.FindStringSubmatch(line)
		if m == nil {
			return nil
		}
		n, err := strconv.ParseInt(m[4], 10, 64)
		if err != nil {
			return fmt.Errorf("memory event count %q: %w", m[4], err)
		}
		// m[3] (the mc:top:mid:low location) is deliberately discarded: it distinguishes csrows
		// of the SAME physical DIMM, and charting per csrow would split one failing stick across
		// two dimensions with no indication they are the same hardware.
		k := dimmKey{dimm: m[2], errType: m[1]}
		if _, seen := memAcc[k]; !seen {
			*memOrder = append(*memOrder, k)
		}
		memAcc[k] += n

	case sectionAER:
		m := reCountTypeMsg.FindStringSubmatch(line)
		if m == nil {
			return nil
		}
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return fmt.Errorf("aer event count %q: %w", m[1], err)
		}
		// Grouped by severity only. The trailing message (m[3]) is intentionally not a label:
		// it is free-form vendor text with unbounded cardinality.
		s.aer = addTyped(s.aer, m[2], n)

	case sectionMCE:
		m := reCountType.FindStringSubmatch(line)
		if m == nil {
			return nil
		}
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return fmt.Errorf("mce event count %q: %w", m[1], err)
		}
		s.mce = addTyped(s.mce, m[2], n)

	case sectionMemoryFailure:
		m := reTypeCount.FindStringSubmatch(line)
		if m == nil {
			return nil
		}
		n, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			return fmt.Errorf("memory failure event count %q: %w", m[2], err)
		}
		s.memoryFailure = addTyped(s.memoryFailure, m[1], n)

	case sectionExtlog:
		return s.addClassCountFirst(classExtlog, line)
	case sectionDevlink:
		return s.addClassDevHas(classDevlink, line)
	case sectionDisk:
		return s.addClassDevHas(classDisk, line)
	case sectionARM:
		return s.addClassDevHas(classARM, line)
	case sectionCXL:
		return s.addClassCountLast(classCXL, line)
	case sectionSignal:
		return s.addClassCountLast(classSignal, line)
	}

	return nil
}

// addClassCountFirst handles "<count> <text> errors".
func (s *summary) addClassCountFirst(class, line string) error {
	m := reCountType.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return fmt.Errorf("%s event count %q: %w", class, m[1], err)
	}
	s.classes[class] += n
	return nil
}

// addClassCountLast handles "<text> errors: <count>".
func (s *summary) addClassCountLast(class, line string) error {
	m := reTypeCount.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	n, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return fmt.Errorf("%s event count %q: %w", class, m[2], err)
	}
	s.classes[class] += n
	return nil
}

// addClassDevHas handles "<device> has <count> errors".
func (s *summary) addClassDevHas(class, line string) error {
	m := reDevCount.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	n, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return fmt.Errorf("%s event count %q: %w", class, m[2], err)
	}
	s.classes[class] += n
	return nil
}

func addTyped(dst []typedEvents, errType string, n int64) []typedEvents {
	errType = strings.TrimSpace(errType)
	for i := range dst {
		if dst[i].errType == errType {
			dst[i].count += n
			return dst
		}
	}
	return append(dst, typedEvents{errType: errType, count: n})
}
