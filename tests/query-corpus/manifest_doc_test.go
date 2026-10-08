// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

type manifestDocumentRow struct {
	proves  string
	cloud   string
	fixedBy string
}

// MANIFEST.md is the human-readable index of what this corpus proves. The
// check covers both membership and every mirrored field, so changing a claim
// in only one ledger cannot pass.
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
	for _, problem := range manifestDocumentProblems(string(raw), manifest) {
		t.Error(problem)
	}
}

func manifestDocumentProblems(body string, cases map[string]ManifestCase) []string {
	var problems []string
	const header = "| Case | Proves | Cloud | Fixed by |"
	tableStart := strings.Index(body, header)
	if tableStart < 0 {
		return []string{fmt.Sprintf("MANIFEST.md is missing the manifest table header %q", header)}
	}
	tableEndOffset := strings.Index(body[tableStart:], "\n## ")
	if tableEndOffset < 0 {
		return []string{"MANIFEST.md is missing a section heading after the manifest table"}
	}
	table := body[tableStart : tableStart+tableEndOffset]

	seen := map[string]int{}
	rows := map[string]manifestDocumentRow{}
	lines := strings.Split(table, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[1]) != "|------|--------|-------|----------|" {
		return []string{"MANIFEST.md is missing the manifest table delimiter row"}
	}
	for _, line := range lines[2:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields, err := splitManifestDocumentRow(line)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		name := fields[0]
		seen[name]++
		if seen[name] == 1 {
			rows[name] = manifestDocumentRow{
				proves:  fields[1],
				cloud:   fields[2],
				fixedBy: fields[3],
			}
		}
	}

	var missing, extra, dupes, mismatched []string
	for name := range cases {
		if seen[name] == 0 {
			missing = append(missing, name)
		}
	}
	for name, n := range seen {
		if _, ok := cases[name]; !ok {
			extra = append(extra, name)
		}
		if n > 1 {
			dupes = append(dupes, name)
		}
	}
	for name, mc := range cases {
		row, ok := rows[name]
		if !ok {
			continue
		}
		cloud := mc.Cloud
		if cloud == "" {
			cloud = "n/a"
		}
		if row.proves != mc.Proves {
			mismatched = append(mismatched, name+"/Proves")
		}
		if row.cloud != cloud {
			mismatched = append(mismatched, name+"/Cloud")
		}
		if row.fixedBy != mc.FixedBy {
			mismatched = append(mismatched, name+"/FixedBy")
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	sort.Strings(dupes)
	sort.Strings(mismatched)

	if len(missing) > 0 {
		problems = append(problems, fmt.Sprintf("MANIFEST.md is missing %d of %d contracts: %s",
			len(missing), len(cases), strings.Join(missing, ", ")))
	}
	if len(extra) > 0 {
		problems = append(problems, fmt.Sprintf("MANIFEST.md documents %d contracts that no longer exist - stale prose "+
			"describing behaviour nothing asserts: %s", len(extra), strings.Join(extra, ", ")))
	}
	if len(dupes) > 0 {
		problems = append(problems, fmt.Sprintf("MANIFEST.md describes %d contracts twice, and two descriptions of one "+
			"contract will disagree: %s", len(dupes), strings.Join(dupes, ", ")))
	}
	if len(mismatched) > 0 {
		problems = append(problems, fmt.Sprintf("MANIFEST.md disagrees with manifest.json in %d field(s): %s",
			len(mismatched), strings.Join(mismatched, ", ")))
	}
	return problems
}

func TestManifestDocumentGuardDetectsDrift(t *testing.T) {
	const valid = `# Manifest

| Case | Proves | Cloud | Fixed by |
|------|--------|-------|----------|
| A | proof with \| pipe | n/a | #1 |

## End
`
	cases := map[string]ManifestCase{
		"A": {Proves: "proof with | pipe", FixedBy: "#1"},
	}
	if problems := manifestDocumentProblems(valid, cases); len(problems) != 0 {
		t.Fatalf("valid document rejected: %v", problems)
	}

	mutations := map[string]struct {
		document string
		want     string
	}{
		"proves": {
			document: strings.Replace(valid, "proof with \\| pipe", "different proof", 1),
			want:     "A/Proves",
		},
		"cloud": {
			document: strings.Replace(valid, "n/a", "cloud", 1),
			want:     "A/Cloud",
		},
		"fixed-by": {
			document: strings.Replace(valid, "#1", "#2", 1),
			want:     "A/FixedBy",
		},
		"missing-row": {
			document: strings.Replace(valid, "| A | proof with \\| pipe | n/a | #1 |\n", "", 1),
			want:     "missing 1 of 1 contracts",
		},
		"extra-row": {
			document: strings.Replace(valid, "\n## End", "| B | ghost | n/a |  |\n\n## End", 1),
			want:     "no longer exist",
		},
		"duplicate-row": {
			document: strings.Replace(valid, "\n## End", "| A | proof with \\| pipe | n/a | #1 |\n\n## End", 1),
			want:     "describes 1 contracts twice",
		},
		"missing-header": {
			document: strings.Replace(valid, "| Case | Proves | Cloud | Fixed by |", "| Wrong | Header |", 1),
			want:     "missing the manifest table header",
		},
		"missing-delimiter": {
			document: strings.Replace(valid, "|------|--------|-------|----------|", "| wrong | delimiter |", 1),
			want:     "missing the manifest table delimiter",
		},
		"row-outside-table": {
			document: strings.Replace(
				strings.Replace(valid, "| A | proof with \\| pipe | n/a | #1 |\n", "", 1),
				"## End", "## End\n\n| A | proof with \\| pipe | n/a | #1 |", 1),
			want: "missing 1 of 1 contracts",
		},
		"missing-end": {
			document: strings.Replace(valid, "\n## End", "", 1),
			want:     "missing a section heading",
		},
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			problems := strings.Join(manifestDocumentProblems(mutation.document, cases), "\n")
			if !strings.Contains(problems, mutation.want) {
				t.Fatalf("mutation was not detected as %q: %s", mutation.want, problems)
			}
		})
	}
}

func splitManifestDocumentRow(line string) ([]string, error) {
	line = strings.TrimSpace(line)
	if len(line) < 2 || line[0] != '|' || line[len(line)-1] != '|' {
		return nil, fmt.Errorf("invalid MANIFEST.md table row %q", line)
	}

	var fields []string
	start := 1
	for i := 1; i < len(line)-1; i++ {
		if line[i] != '|' || manifestPipeEscaped(line, i) {
			continue
		}
		fields = append(fields, unescapeManifestField(line[start:i]))
		start = i + 1
	}
	fields = append(fields, unescapeManifestField(line[start:len(line)-1]))
	if len(fields) != 4 {
		return nil, fmt.Errorf("MANIFEST.md row has %d fields, want 4: %q", len(fields), line)
	}
	return fields, nil
}

func manifestPipeEscaped(line string, pipe int) bool {
	backslashes := 0
	for i := pipe - 1; i >= 0 && line[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func unescapeManifestField(field string) string {
	return strings.ReplaceAll(strings.TrimSpace(field), `\|`, `|`)
}
