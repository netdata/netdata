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
// the next reader. This keeps it honest without taking the prose away.
func TestManifestDocumentIsComplete(t *testing.T) {
	raw, err := os.ReadFile("MANIFEST.md")
	if err != nil {
		t.Fatal(err)
	}

	documented := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\| ([^|]+?) \|`).FindAllStringSubmatch(string(raw), -1) {
		documented[strings.TrimSpace(m[1])] = true
	}

	var missing []string
	for name := range manifest {
		if !documented[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Errorf("MANIFEST.md is missing %d of %d contracts: %s",
			len(missing), len(manifest), strings.Join(missing, ", "))
	}
}
