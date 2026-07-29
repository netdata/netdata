// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// MANIFEST.md is the human-readable index of what this corpus proves. It is
// written by hand, which means it drifts - it has done so twice, and a
// ledger describing a defect as the contract is how a fix gets reverted by
// the next reader.
//
// The check is deliberately three-sided. Missing rows hide a contract;
// EXTRA rows are worse, because a row for a contract that no longer exists
// is stale prose describing behaviour nobody asserts any more; and a
// DUPLICATE row means two descriptions of one contract, which will disagree
// eventually. Rows are only counted inside the table, so a row appended
// past the end of it does not satisfy the check by being somewhere in the
// file.
func TestManifestDocumentMatchesContracts(t *testing.T) {
	raw, err := os.ReadFile("MANIFEST.md")
	if err != nil {
		t.Fatal(err)
	}

	// the table ends where the prose sections begin
	body := string(raw)
	if end := strings.Index(body, "\n## "); end > 0 {
		if tbl := strings.Index(body, "| Case | Proves |"); tbl >= 0 && tbl < end {
			body = body[tbl:end]
		}
	}

	seen := map[string]int{}
	for _, m := range regexp.MustCompile(`(?m)^\| ([^|]+?) \|`).FindAllStringSubmatch(body, -1) {
		name := strings.TrimSpace(m[1])
		if name == "Case" || strings.HasPrefix(name, "---") {
			continue
		}
		seen[name]++
	}

	var missing, extra, dupes []string
	for name := range manifest {
		if seen[name] == 0 {
			missing = append(missing, name)
		}
	}
	for name, n := range seen {
		if _, ok := manifest[name]; !ok {
			extra = append(extra, name)
		}
		if n > 1 {
			dupes = append(dupes, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	sort.Strings(dupes)

	if len(missing) > 0 {
		t.Errorf("MANIFEST.md is missing %d of %d contracts: %s",
			len(missing), len(manifest), strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		t.Errorf("MANIFEST.md documents %d contracts that no longer exist - stale prose "+
			"describing behaviour nothing asserts: %s", len(extra), strings.Join(extra, ", "))
	}
	if len(dupes) > 0 {
		t.Errorf("MANIFEST.md describes %d contracts twice, and two descriptions of one "+
			"contract will disagree: %s", len(dupes), strings.Join(dupes, ", "))
	}
}
