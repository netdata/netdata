package agentdriver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/perffixture"
)

func TestOptionsExposeOnlyPublicModeFixtureAndTransportValues(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, perffixture.ModuleName+".conf"), perffixture.ConfigYAML(), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, err := parseOptions([]string{
		"--mode=wire/agent", "--fixture-config-dir=" + directory,
		"--event-fd=3", "--process-generation=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.mode != wireAgentMode || opts.fixtureConfigDir != directory || opts.eventFD != 3 || opts.processGeneration != 1 {
		t.Fatalf("options differ: %#v", opts)
	}
	for _, args := range [][]string{
		{"--mode=wire/agent", "--fixture-config-dir=" + directory, "--event-fd=3", "--process-generation=1", "--case-id=F01.1"},
		{"--mode=wire/cut", "--fixture-config-dir=" + directory, "--event-fd=3", "--process-generation=1"},
		{"--mode=wire/agent", "--fixture-config-dir=relative", "--event-fd=3", "--process-generation=1"},
	} {
		if _, err := parseOptions(args); err == nil {
			t.Fatalf("invalid child arguments accepted: %q", args)
		}
	}
}
