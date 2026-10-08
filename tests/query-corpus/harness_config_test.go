// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCorpusPathFixture(t *testing.T) (base, binary, source string) {
	t.Helper()
	root := t.TempDir()
	base = filepath.Join(root, "tests", "query-corpus")
	binary = filepath.Join(root, "build", "netdata")
	source = filepath.Join(root, "src")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{
		"web/api/queries/query.h",
		"web/api/queries/query-group-over-time.c",
	} {
		path := filepath.Join(source, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return base, binary, source
}

func TestResolveCorpusPathsRequiresPairedOverrides(t *testing.T) {
	base, binary, source := writeCorpusPathFixture(t)

	for name, overrides := range map[string][2]string{
		"binary only": {binary, ""},
		"source only": {"", source},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := resolveCorpusPaths(overrides[0], overrides[1], base)
			if err == nil || !strings.Contains(err.Error(), "together") {
				t.Fatalf("resolveCorpusPaths() error = %v, want paired-override rejection", err)
			}
		})
	}
}

func TestResolveCorpusPathsValidatesOneDeclaredEngine(t *testing.T) {
	base, binary, source := writeCorpusPathFixture(t)

	got, err := resolveCorpusPaths(binary, source, base)
	if err != nil {
		t.Fatal(err)
	}
	if got.Binary != binary || got.Source != source {
		t.Fatalf("resolved paths = %+v, want %q and %q", got, binary, source)
	}

	got, err = resolveCorpusPaths("", "", base)
	if err != nil {
		t.Fatal(err)
	}
	if got.Binary != filepath.Clean(filepath.Join(base, "../../build/netdata")) ||
		got.Source != filepath.Clean(filepath.Join(base, "../../src")) {
		t.Fatalf("default paths = %+v", got)
	}
}

func TestResolveCorpusPathsRejectsUnusablePair(t *testing.T) {
	base, binary, source := writeCorpusPathFixture(t)

	if err := os.Chmod(binary, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveCorpusPaths(binary, source, base); err == nil {
		t.Fatal("non-executable binary accepted")
	}

	if err := os.Chmod(binary, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(source, "web/api/queries/query-group-over-time.c")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveCorpusPaths(binary, source, base); err == nil {
		t.Fatal("incomplete source tree accepted")
	}
}

func TestDaemonRunRequiredFailsClosed(t *testing.T) {
	tests := map[string]struct {
		run, list string
		want      bool
	}{
		"full run":              {want: true},
		"listing":               {list: ".", want: false},
		"empty selection":       {run: "^$", want: false},
		"empty selection alt":   {run: "$^", want: false},
		"pure exact":            {run: "^TestResolveCorpusPathsValidatesOneDeclaredEngine$", want: false},
		"pure unanchored":       {run: "TestContractLedgerDeduplicatesAndKeepsFailuresSticky", want: true},
		"allowlisted prefix":    {run: "^TestManifestDocumentAgainstDaemon$", want: true},
		"pure C4D guard":        {run: "^TestC4DAlignedGridOracleGuardsOffByOne$", want: false},
		"pure C023 cadence":     {run: "^TestC023ResolutionWindowRecordsUseFixtureCadence$", want: false},
		"runtime exact":         {run: "^TestLayer0RoundTrip$", want: true},
		"mixed alternation":     {run: "TestContractLedger|TestLayer0RoundTrip", want: true},
		"broad regular expr":    {run: "Test.*", want: true},
		"unknown simple prefix": {run: "TestFutureHarnessGuard", want: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := daemonRunRequired(tc.run, tc.list); got != tc.want {
				t.Fatalf("daemonRunRequired(%q,%q) = %v, want %v", tc.run, tc.list, got, tc.want)
			}
		})
	}
}
