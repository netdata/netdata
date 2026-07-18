package wireeval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBaselineBundleVerifiesExactFilesAndPlatform(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "fixture"), 0o750); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(directory, "currentreal")
	fixture := filepath.Join(directory, "fixture", "perf.conf")
	if err := os.WriteFile(executable, []byte("baseline"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	executableSHA256, err := fileSHA256(executable)
	if err != nil {
		t.Fatal(err)
	}
	fixtureSHA256, err := fileSHA256(fixture)
	if err != nil {
		t.Fatal(err)
	}
	metadata := baselineMetadata{
		Schema:              "jobmgr-bm001-baseline-metadata-v1",
		CurrentBinarySHA256: executableSHA256,
		FixtureSHA256:       fixtureSHA256,
	}
	metadata.Environment.GoVersion = runtime.Version()
	metadata.Environment.GOOS = runtime.GOOS
	metadata.Environment.GOARCH = runtime.GOARCH
	metadata.Environment.GOMAXPROCS = 4
	metadataFile, err := os.OpenFile(
		filepath.Join(directory, "metadata.json"),
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0o600,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(metadataFile).Encode(metadata); err != nil {
		_ = metadataFile.Close()
		t.Fatal(err)
	}
	if err := metadataFile.Close(); err != nil {
		t.Fatal(err)
	}
	metadataSHA256, err := fileSHA256(filepath.Join(directory, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := executableSHA256 + "  currentreal\n" +
		fixtureSHA256 + "  fixture/perf.conf\n" +
		metadataSHA256 + "  metadata.json\n"
	if err := os.WriteFile(
		filepath.Join(directory, "CHECKSUMS.sha256"),
		[]byte(manifest),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	bundle, err := VerifyBaselineBundle(directory)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Executable != executable ||
		bundle.FixtureDir != filepath.Join(directory, "fixture") ||
		bundle.MetadataSHA256 != metadataSHA256 {
		t.Fatalf("verified bundle differs: %#v", bundle)
	}
	if err := os.WriteFile(fixture, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyBaselineBundle(directory); err == nil {
		t.Fatal("mutated fixture passed baseline verification")
	}
}

func TestBaselineManifestRejectsUnsafePaths(t *testing.T) {
	tests := map[string]string{
		"parent":   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  ../outside\n",
		"absolute": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  /outside\n",
		"duplicate": "" +
			"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  file\n" +
			"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  file\n",
	}
	for name, manifest := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "CHECKSUMS.sha256")
			if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readChecksumManifest(path); err == nil {
				t.Fatal("unsafe checksum manifest was accepted")
			}
		})
	}
}
