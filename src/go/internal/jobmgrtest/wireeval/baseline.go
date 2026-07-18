package wireeval

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type BaselineBundle struct {
	Executable     string
	FixtureDir     string
	MetadataSHA256 string
}

type baselineMetadata struct {
	Schema string `json:"schema"`
	Source struct {
		HeadCommit string `json:"head_commit"`
		HeadTree   string `json:"head_tree"`
	} `json:"source"`
	Environment struct {
		GoVersion  string `json:"go_version"`
		GOOS       string `json:"goos"`
		GOARCH     string `json:"goarch"`
		GOMAXPROCS int    `json:"gomaxprocs"`
	} `json:"environment"`
	CurrentBinarySHA256     string `json:"current_binary_sha256"`
	PerformanceDriverSHA256 string `json:"performance_driver_sha256"`
	FixtureSHA256           string `json:"fixture_sha256"`
}

type baselinePlatform struct {
	GOOS      string
	GOARCH    string
	GoVersion string
}

type baselineIdentityContract struct {
	SourceRevision          string
	SourceTree              string
	PerformanceDriverSHA256 string
	FixtureSHA256           string
	Executables             map[baselinePlatform]string
}

var step0BaselineIdentity = baselineIdentityContract{
	SourceRevision:          "8f01d7ec33055303fc6b02f76871bede18fe3d68",
	SourceTree:              "c197c98936bb525f3e6d1d32ffe389badbabae4f",
	PerformanceDriverSHA256: "4c2473e33e3f1299438869d42ba72d5bfdc6e61473c27685b5ccffcb89ef8a4e",
	FixtureSHA256:           "46189fd9f245e945c91ab16709c200511767ca2a97bfc04cb37c32224c6fa5fc",
	Executables: map[baselinePlatform]string{
		{
			GOOS: "darwin", GOARCH: "arm64", GoVersion: "go1.26.5",
		}: "6d359989e417809e85182d6764d356e70b709242c15ece107a114096cb70825e",
		{
			GOOS: "linux", GOARCH: "amd64", GoVersion: "go1.26.5",
		}: "7eafaa2bf9a3a5b186250a9a90273eb04e490c740a188fa44f14b38590f085a0",
		{
			GOOS: "linux", GOARCH: "arm64", GoVersion: "go1.26.5",
		}: "40be5dafebc3c93d6ac35a0183228c2f39a223ec6a382903eeeabef1c014dde8",
	},
}

func VerifyBaselineBundle(directory string) (BaselineBundle, error) {
	return verifyBaselineBundle(directory, step0BaselineIdentity)
}

func verifyBaselineBundle(
	directory string,
	identity baselineIdentityContract,
) (BaselineBundle, error) {
	if !filepath.IsAbs(directory) {
		return BaselineBundle{}, errors.New("wire evaluator: baseline directory must be absolute")
	}
	checksums, err := readChecksumManifest(
		filepath.Join(directory, "CHECKSUMS.sha256"),
	)
	if err != nil {
		return BaselineBundle{}, err
	}
	for name, checksum := range checksums {
		if err := verifyFileSHA256(filepath.Join(directory, name), checksum); err != nil {
			return BaselineBundle{}, err
		}
	}
	required := []string{"currentreal", "fixture/perf.conf", "metadata.json"}
	for _, name := range required {
		if checksums[name] == "" {
			return BaselineBundle{}, fmt.Errorf(
				"wire evaluator: baseline manifest lacks %s",
				name,
			)
		}
	}
	var metadata baselineMetadata
	if err := readJSONFile(filepath.Join(directory, "metadata.json"), &metadata); err != nil {
		return BaselineBundle{}, err
	}
	if metadata.Schema != "jobmgr-bm001-baseline-metadata-v1" {
		return BaselineBundle{}, errors.New("wire evaluator: unsupported baseline metadata")
	}
	if metadata.Environment.GOOS != runtime.GOOS ||
		metadata.Environment.GOARCH != runtime.GOARCH ||
		metadata.Environment.GoVersion != runtime.Version() ||
		metadata.Environment.GOMAXPROCS != 4 {
		return BaselineBundle{}, fmt.Errorf(
			"wire evaluator: baseline platform %s/%s %s GOMAXPROCS=%d differs from host %s/%s %s GOMAXPROCS=4",
			metadata.Environment.GOOS,
			metadata.Environment.GOARCH,
			metadata.Environment.GoVersion,
			metadata.Environment.GOMAXPROCS,
			runtime.GOOS,
			runtime.GOARCH,
			runtime.Version(),
		)
	}
	if metadata.CurrentBinarySHA256 != checksums["currentreal"] ||
		metadata.FixtureSHA256 != checksums["fixture/perf.conf"] {
		return BaselineBundle{}, errors.New("wire evaluator: baseline metadata digest differs")
	}
	platform := baselinePlatform{
		GOOS:      metadata.Environment.GOOS,
		GOARCH:    metadata.Environment.GOARCH,
		GoVersion: metadata.Environment.GoVersion,
	}
	executableSHA256 := identity.Executables[platform]
	if executableSHA256 == "" {
		return BaselineBundle{}, fmt.Errorf(
			"wire evaluator: baseline platform identity is not pinned: %s/%s %s",
			platform.GOOS,
			platform.GOARCH,
			platform.GoVersion,
		)
	}
	if metadata.Source.HeadCommit != identity.SourceRevision ||
		metadata.Source.HeadTree != identity.SourceTree ||
		metadata.PerformanceDriverSHA256 !=
			identity.PerformanceDriverSHA256 ||
		metadata.FixtureSHA256 != identity.FixtureSHA256 ||
		metadata.CurrentBinarySHA256 != executableSHA256 {
		return BaselineBundle{}, errors.New(
			"wire evaluator: baseline identity differs from compiled Step-0 contract",
		)
	}
	executable := filepath.Join(directory, "currentreal")
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return BaselineBundle{}, errors.New("wire evaluator: baseline executable is unavailable")
	}
	return BaselineBundle{
		Executable:     executable,
		FixtureDir:     filepath.Join(directory, "fixture"),
		MetadataSHA256: checksums["metadata.json"],
	}, nil
}

func readChecksumManifest(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	checksums := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 || len(parts[0]) != 64 {
			return nil, errors.New("wire evaluator: malformed baseline checksum")
		}
		name := filepath.Clean(parts[1])
		if filepath.IsAbs(name) || name == "." || name == ".." ||
			strings.HasPrefix(name, ".."+string(filepath.Separator)) ||
			filepath.ToSlash(name) != parts[1] {
			return nil, errors.New("wire evaluator: unsafe baseline checksum path")
		}
		if _, exists := checksums[name]; exists {
			return nil, errors.New("wire evaluator: duplicate baseline checksum path")
		}
		checksums[name] = parts[0]
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(checksums) == 0 {
		return nil, errors.New("wire evaluator: empty baseline checksum manifest")
	}
	return checksums, nil
}
