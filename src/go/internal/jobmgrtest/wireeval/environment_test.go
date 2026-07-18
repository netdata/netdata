package wireeval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/perffixture"
)

func TestEnvironmentDigestIsDerivedFromExactPairInputs(t *testing.T) {
	fixtureDir := t.TempDir()
	fixturePath := filepath.Join(fixtureDir, perffixture.ModuleName+".conf")
	baselinePath := filepath.Join(t.TempDir(), "baseline")
	productionPath := filepath.Join(t.TempDir(), "production")
	for path, payload := range map[string][]byte{
		fixturePath:    perffixture.ConfigYAML(),
		baselinePath:   []byte("baseline executable"),
		productionPath: []byte("production executable"),
	} {
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	child := func(executable string) ChildSpec {
		return ChildSpec{
			Executable: executable,
			Arguments: []string{
				"--mode=wire/agent",
				"--fixture-config-dir=" + fixtureDir,
			},
		}
	}
	first, err := DeriveEnvironmentSHA256(child(baselinePath), child(productionPath))
	if err != nil {
		t.Fatal(err)
	}
	second, err := DeriveEnvironmentSHA256(child(baselinePath), child(productionPath))
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("environment digest is not stable: %q %q", first, second)
	}
	if err := os.WriteFile(productionPath, []byte("changed production executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := DeriveEnvironmentSHA256(child(baselinePath), child(productionPath))
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("changed production executable retained the environment digest")
	}
}

func TestEnvironmentDigestRejectsDifferentFixtureDirectories(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "executable")
	if err := os.WriteFile(executable, []byte("executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	directories := []string{t.TempDir(), t.TempDir()}
	for _, directory := range directories {
		if err := os.WriteFile(
			filepath.Join(directory, perffixture.ModuleName+".conf"),
			perffixture.ConfigYAML(),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := DeriveEnvironmentSHA256(
		ChildSpec{
			Executable: executable,
			Arguments:  []string{"--fixture-config-dir=" + directories[0]},
		},
		ChildSpec{
			Executable: executable,
			Arguments:  []string{"--fixture-config-dir=" + directories[1]},
		},
	); err == nil {
		t.Fatal("different pair fixture directories were accepted")
	}
}
