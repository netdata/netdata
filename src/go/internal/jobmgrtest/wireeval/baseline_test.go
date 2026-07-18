package wireeval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStep0BaselineIdentityPinsApprovedPlatforms(t *testing.T) {
	expected := map[baselinePlatform]string{
		{
			GOOS: "darwin", GOARCH: "arm64", GoVersion: "go1.26.5",
		}: "6d359989e417809e85182d6764d356e70b709242c15ece107a114096cb70825e",
		{
			GOOS: "linux", GOARCH: "amd64", GoVersion: "go1.26.5",
		}: "7eafaa2bf9a3a5b186250a9a90273eb04e490c740a188fa44f14b38590f085a0",
		{
			GOOS: "linux", GOARCH: "arm64", GoVersion: "go1.26.5",
		}: "40be5dafebc3c93d6ac35a0183228c2f39a223ec6a382903eeeabef1c014dde8",
	}
	if len(step0BaselineIdentity.Executables) != len(expected) {
		t.Fatalf(
			"approved baseline platforms=%d, want %d",
			len(step0BaselineIdentity.Executables),
			len(expected),
		)
	}
	for platform, want := range expected {
		if got := step0BaselineIdentity.Executables[platform]; got != want {
			t.Fatalf(
				"baseline %s/%s %s digest=%q, want %q",
				platform.GOOS,
				platform.GOARCH,
				platform.GoVersion,
				got,
				want,
			)
		}
	}
}

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
		Schema:                  "jobmgr-bm001-baseline-metadata-v1",
		CurrentBinarySHA256:     executableSHA256,
		FixtureSHA256:           fixtureSHA256,
		PerformanceDriverSHA256: "driver",
	}
	metadata.Source.HeadCommit = "revision"
	metadata.Source.HeadTree = "tree"
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
	identity := baselineIdentityContract{
		SourceRevision:          metadata.Source.HeadCommit,
		SourceTree:              metadata.Source.HeadTree,
		PerformanceDriverSHA256: metadata.PerformanceDriverSHA256,
		FixtureSHA256:           fixtureSHA256,
		Executables: map[baselinePlatform]string{
			{
				GOOS:      runtime.GOOS,
				GOARCH:    runtime.GOARCH,
				GoVersion: runtime.Version(),
			}: executableSHA256,
		},
	}
	bundle, err := verifyBaselineBundle(directory, identity)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Executable != executable ||
		bundle.FixtureDir != filepath.Join(directory, "fixture") ||
		bundle.MetadataSHA256 != metadataSHA256 {
		t.Fatalf("verified bundle differs: %#v", bundle)
	}
	if _, err := VerifyBaselineBundle(directory); err == nil {
		t.Fatal(
			"caller-consistent bundle passed the compiled Step-0 identity",
		)
	}
	if err := os.WriteFile(fixture, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyBaselineBundle(directory, identity); err == nil {
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
