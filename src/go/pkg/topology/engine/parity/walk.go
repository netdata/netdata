// SPDX-License-Identifier: GPL-3.0-or-later

package parity

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var snmpWalkLineRE = regexp.MustCompile(`^\s*\.?([0-9][0-9.]*)\s*=\s*([^:]+):\s*(.*)$`)

// WalkRecord is one normalized SNMP walk row.
type WalkRecord struct {
	OID   string `json:"oid" yaml:"oid"`
	Type  string `json:"type" yaml:"type"`
	Value string `json:"value" yaml:"value"`
}

// WalkDataset is a parsed SNMP walk fixture with deterministic ordering.
type WalkDataset struct {
	Path    string       `json:"path" yaml:"path"`
	Records []WalkRecord `json:"records" yaml:"records"`
	byOID   map[string]WalkRecord
}

// LoadWalkFile parses a walk file in OpenNMS snmpwalk format.
func LoadWalkFile(path string) (WalkDataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return WalkDataset{}, fmt.Errorf("open walk file %q: %w", path, err)
	}

	records, err := ParseWalk(f)
	if closeErr := f.Close(); err == nil && closeErr != nil {
		return WalkDataset{}, fmt.Errorf("close walk file %q: %w", path, closeErr)
	}
	if err != nil {
		return WalkDataset{}, fmt.Errorf("parse walk file %q: %w", path, err)
	}

	ds := WalkDataset{Path: path, Records: records, byOID: make(map[string]WalkRecord, len(records))}
	for _, rec := range records {
		ds.byOID[rec.OID] = rec
	}
	return ds, nil
}

// ParseWalk parses OpenNMS snmpwalk text records.
func ParseWalk(r io.Reader) ([]WalkRecord, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	type partial struct {
		oid       string
		typ       string
		value     strings.Builder
		quoteOpen bool
		escaped   bool
	}

	var (
		lineNo  int
		records []WalkRecord
		cur     *partial
	)

	flush := func() {
		if cur == nil {
			return
		}
		records = append(records, WalkRecord{
			OID:   cur.oid,
			Type:  cur.typ,
			Value: normalizeWalkValue(cur.value.String()),
		})
		cur = nil
	}

	for scanner.Scan() {
		lineNo++
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			if cur != nil {
				cur.value.WriteByte('\n')
				cur.quoteOpen, cur.escaped = updateQuoteState("\n", cur.quoteOpen, cur.escaped)
			}
			continue
		}

		if cur != nil {
			if cur.value.Len() > 0 {
				cur.value.WriteByte('\n')
				cur.quoteOpen, cur.escaped = updateQuoteState("\n", cur.quoteOpen, cur.escaped)
			}
			cur.value.WriteString(line)
			cur.quoteOpen, cur.escaped = updateQuoteState(line, cur.quoteOpen, cur.escaped)
			if !cur.quoteOpen {
				cur.quoteOpen = false
				flush()
			}
			continue
		}

		match := snmpWalkLineRE.FindStringSubmatch(line)
		if len(match) != 4 {
			// Ignore non-record lines; these appear in a few vendor dumps.
			continue
		}

		oid := normalizeOID(match[1])
		typ := strings.TrimSpace(match[2])
		value := match[3]
		if oid == "" || typ == "" {
			continue
		}

		quoteOpen, escaped := updateQuoteState(value, false, false)
		if quoteOpen {
			cur = &partial{oid: oid, typ: typ, quoteOpen: quoteOpen, escaped: escaped}
			cur.value.WriteString(value)
			continue
		}

		records = append(records, WalkRecord{
			OID:   oid,
			Type:  typ,
			Value: normalizeWalkValue(value),
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan walk line %d: %w", lineNo, err)
	}
	if cur != nil && cur.quoteOpen {
		return nil, fmt.Errorf("unterminated quoted value for OID %s", cur.oid)
	}
	return records, nil
}

// Lookup returns one record by OID. OID matching ignores leading dots.
func (d *WalkDataset) Lookup(oid string) (WalkRecord, bool) {
	if d == nil {
		return WalkRecord{}, false
	}
	if len(d.byOID) == 0 {
		index := make(map[string]WalkRecord, len(d.Records))
		for _, rec := range d.Records {
			index[rec.OID] = rec
		}
		d.byOID = index
	}
	rec, ok := d.byOID[normalizeOID(oid)]
	return rec, ok
}

// Prefix returns all records with the given OID prefix, in file order.
func (d WalkDataset) Prefix(prefix string) []WalkRecord {
	norm := normalizeOID(prefix)
	if norm == "" {
		out := make([]WalkRecord, len(d.Records))
		copy(out, d.Records)
		return out
	}
	out := make([]WalkRecord, 0)
	for _, rec := range d.Records {
		if strings.HasPrefix(rec.OID, norm) {
			out = append(out, rec)
		}
	}
	return out
}

// SortedOIDs returns deterministic OID ordering from the dataset.
func (d WalkDataset) SortedOIDs() []string {
	oids := make([]string, 0, len(d.Records))
	for _, rec := range d.Records {
		oids = append(oids, rec.OID)
	}
	sort.Slice(oids, func(i, j int) bool {
		return compareOID(oids[i], oids[j]) < 0
	})
	return oids
}

func compareOID(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")
	limit := len(partsA)
	if len(partsB) < limit {
		limit = len(partsB)
	}

	for i := 0; i < limit; i++ {
		if partsA[i] == partsB[i] {
			continue
		}

		numA, errA := strconv.Atoi(partsA[i])
		numB, errB := strconv.Atoi(partsB[i])
		if errA == nil && errB == nil {
			switch {
			case numA < numB:
				return -1
			case numA > numB:
				return 1
			default:
				continue
			}
		}

		if partsA[i] < partsB[i] {
			return -1
		}
		return 1
	}

	switch {
	case len(partsA) < len(partsB):
		return -1
	case len(partsA) > len(partsB):
		return 1
	default:
		return 0
	}
}

func normalizeOID(v string) string {
	return strings.TrimLeft(strings.TrimSpace(v), ".")
}

func normalizeWalkValue(v string) string {
	s := strings.TrimSpace(v)
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		s = strings.TrimPrefix(s, `"`)
		s = strings.TrimSuffix(s, `"`)
	}
	s = strings.ReplaceAll(s, `\"`, `"`)
	return s
}

func updateQuoteState(v string, quoteOpen, escaped bool) (bool, bool) {
	for i := 0; i < len(v); i++ {
		ch := v[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			quoteOpen = !quoteOpen
		}
	}
	return quoteOpen, escaped
}
