package wireeval

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/perffixture"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/runner"
)

func DeriveEnvironmentSHA256(baseline, production ChildSpec) (string, error) {
	digest := sha256.New()
	writeEnvironmentString(digest, "jobmgrtest-performance-environment-v1")
	for _, side := range []struct {
		name string
		spec ChildSpec
	}{
		{name: "baseline", spec: baseline},
		{name: "production", spec: production},
	} {
		if err := appendChildIdentity(digest, side.name, side.spec); err != nil {
			return "", err
		}
	}
	fixtureDir, err := sharedFixtureDirectory(baseline, production)
	if err != nil {
		return "", err
	}
	if err := appendFileIdentity(
		digest,
		"fixture",
		filepath.Join(fixtureDir, perffixture.ModuleName+".conf"),
	); err != nil {
		return "", err
	}
	writeEnvironmentString(digest, runtime.GOOS)
	writeEnvironmentString(digest, runtime.GOARCH)
	writeEnvironmentString(digest, runtime.Version())
	writeEnvironmentString(digest, fmt.Sprintf("gomaxprocs=%d", runtime.GOMAXPROCS(0)))
	writeEnvironmentString(digest, fmt.Sprintf("numcpu=%d", runtime.NumCPU()))
	for _, value := range runner.ChildEnvironment() {
		writeEnvironmentString(digest, value)
	}
	cpu, err := cpuIdentity()
	if err != nil {
		return "", err
	}
	writeEnvironmentString(digest, cpu)
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func appendChildIdentity(digest hash.Hash, side string, spec ChildSpec) error {
	if !filepath.IsAbs(spec.Executable) {
		return fmt.Errorf("wire evaluator: %s executable must be absolute", side)
	}
	writeEnvironmentString(digest, side)
	if err := appendFileIdentity(digest, side+"-executable", spec.Executable); err != nil {
		return err
	}
	for _, argument := range spec.Arguments {
		writeEnvironmentString(digest, argument)
	}
	if spec.CutControl {
		writeEnvironmentString(digest, "cut-control=true")
	} else {
		writeEnvironmentString(digest, "cut-control=false")
	}
	return nil
}

func appendFileIdentity(digest hash.Hash, label, path string) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("wire evaluator: read %s: %w", label, err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("wire evaluator: %s is not a regular file", label)
	}
	sum := sha256.Sum256(payload)
	writeEnvironmentString(digest, label)
	writeEnvironmentString(digest, hex.EncodeToString(sum[:]))
	return nil
}

func sharedFixtureDirectory(baseline, production ChildSpec) (string, error) {
	baselineDir, err := fixtureDirectory(baseline.Arguments)
	if err != nil {
		return "", fmt.Errorf("wire evaluator: baseline: %w", err)
	}
	productionDir, err := fixtureDirectory(production.Arguments)
	if err != nil {
		return "", fmt.Errorf("wire evaluator: production: %w", err)
	}
	if baselineDir != productionDir {
		return "", errors.New("wire evaluator: pair sides use different fixture directories")
	}
	return baselineDir, nil
}

func fixtureDirectory(arguments []string) (string, error) {
	const prefix = "--fixture-config-dir="
	var directory string
	for _, argument := range arguments {
		if !strings.HasPrefix(argument, prefix) {
			continue
		}
		if directory != "" {
			return "", errors.New("duplicate fixture config directory")
		}
		directory = strings.TrimPrefix(argument, prefix)
	}
	if !filepath.IsAbs(directory) {
		return "", errors.New("fixture config directory must be absolute")
	}
	return directory, nil
}

func cpuIdentity() (string, error) {
	switch runtime.GOOS {
	case "linux":
		payload, err := os.ReadFile("/proc/cpuinfo")
		if err != nil {
			return "", fmt.Errorf("wire evaluator: read CPU identity: %w", err)
		}
		sum := sha256.Sum256(payload)
		return "linux-cpuinfo-sha256=" + hex.EncodeToString(sum[:]), nil
	case "darwin":
		payload, err := exec.Command("/usr/sbin/sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err != nil {
			return "", fmt.Errorf("wire evaluator: read CPU identity: %w", err)
		}
		sum := sha256.Sum256(payload)
		return "darwin-cpu-brand-sha256=" + hex.EncodeToString(sum[:]), nil
	default:
		return fmt.Sprintf("%s/%s/numcpu=%d", runtime.GOOS, runtime.GOARCH, runtime.NumCPU()), nil
	}
}

func writeEnvironmentString(digest hash.Hash, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = digest.Write(size[:])
	_, _ = digest.Write([]byte(value))
}
