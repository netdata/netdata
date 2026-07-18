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
	Schema      string `json:"schema"`
	Environment struct {
		GoVersion  string `json:"go_version"`
		GOOS       string `json:"goos"`
		GOARCH     string `json:"goarch"`
		GOMAXPROCS int    `json:"gomaxprocs"`
	} `json:"environment"`
	CurrentBinarySHA256 string `json:"current_binary_sha256"`
	FixtureSHA256       string `json:"fixture_sha256"`
}

func VerifyBaselineBundle(directory string) (BaselineBundle, error) {
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
