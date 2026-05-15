// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

type ieeeSource struct {
	name string
	url  string
}

var ieeeSources = []ieeeSource{
	{name: "ieee_oui", url: "https://standards-oui.ieee.org/oui/oui.csv"},
	{name: "ieee_cid", url: "https://standards-oui.ieee.org/cid/cid.csv"},
	{name: "ieee_oui36", url: "https://standards-oui.ieee.org/oui36/oui36.csv"},
	{name: "ieee_iab", url: "https://standards-oui.ieee.org/iab/iab.csv"},
}

var nonHex = regexp.MustCompile(`[^0-9A-Fa-f]`)
var httpClient = &http.Client{Timeout: 30 * time.Second}

func main() {
	var outputPath string
	flag.StringVar(&outputPath, "out", "", "output TSV path (required)")
	flag.Parse()

	if strings.TrimSpace(outputPath) == "" {
		fmt.Fprintln(os.Stderr, "missing required -out argument")
		os.Exit(2)
	}

	index, err := buildVendorIndex()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build OUI index: %v\n", err)
		os.Exit(1)
	}
	if err := writeDataset(outputPath, index); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write dataset: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d entries to %s\n", len(index), outputPath)
}

func buildVendorIndex() (map[string]string, error) {
	prefixToVendor := make(map[string]string, 120000)

	for _, src := range ieeeSources {
		records, err := fetchCSV(src.url)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", src.name, err)
		}
		for i, record := range records {
			if i == 0 { // header
				continue
			}
			if len(record) < 3 {
				continue
			}
			registry := strings.TrimSpace(record[0])
			assignment := normalizeAssignment(record[1])
			vendor := strings.TrimSpace(record[2])
			if assignment == "" || vendor == "" {
				continue
			}
			prefixLen := prefixLengthForRegistry(registry, assignment)
			if prefixLen == 0 || len(assignment) < prefixLen {
				continue
			}
			prefix := assignment[:prefixLen]

			if existing, ok := prefixToVendor[prefix]; ok {
				// Prefer the more descriptive name on conflicts.
				if len(existing) >= len(vendor) {
					continue
				}
			}
			prefixToVendor[prefix] = vendor
		}
	}
	return prefixToVendor, nil
}

func fetchCSV(url string) ([][]string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "netdata-topology-oui-dataset-updater/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	reader := csv.NewReader(resp.Body)
	reader.FieldsPerRecord = -1
	records := make([][]string, 0, 65536)
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func normalizeAssignment(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = nonHex.ReplaceAllString(raw, "")
	raw = strings.ToUpper(raw)
	if len(raw) > 12 {
		raw = raw[:12]
	}
	return raw
}

func prefixLengthForRegistry(registry, assignment string) int {
	switch strings.ToUpper(strings.TrimSpace(registry)) {
	case "MA-S", "IAB":
		if len(assignment) >= 9 {
			return 9
		}
	case "MA-M":
		if len(assignment) >= 7 {
			return 7
		}
	case "MA-L", "CID":
		if len(assignment) >= 6 {
			return 6
		}
	}
	// Fallback for non-standard/unknown registry values.
	if len(assignment) >= 9 {
		return 9
	}
	if len(assignment) >= 7 {
		return 7
	}
	if len(assignment) >= 6 {
		return 6
	}
	return 0
}

func writeDataset(path string, index map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriterSize(f, 1<<20)
	defer w.Flush()

	prefixes := make([]string, 0, len(index))
	for prefix := range index {
		prefixes = append(prefixes, prefix)
	}
	sort.Slice(prefixes, func(i, j int) bool {
		if len(prefixes[i]) != len(prefixes[j]) {
			return len(prefixes[i]) > len(prefixes[j])
		}
		return prefixes[i] < prefixes[j]
	})

	fmt.Fprintf(w, "# Netdata Topology OUI Vendor Dataset\n")
	fmt.Fprintf(w, "# Generated: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(w, "# Sources:\n")
	for _, src := range ieeeSources {
		fmt.Fprintf(w, "# - %s %s\n", src.name, src.url)
	}
	fmt.Fprintf(w, "# Format: <HEX_PREFIX>\\t<VENDOR>\n")

	for _, prefix := range prefixes {
		fmt.Fprintf(w, "%s\t%s\n", prefix, index[prefix])
	}
	return nil
}
