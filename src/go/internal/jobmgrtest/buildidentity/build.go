package buildidentity

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const maximumExecutableBytes = 512 * 1024 * 1024

type BuildTarget struct {
	ImportPath  string
	Expectation ExecutableExpectation
}

func BuildExecutable(
	ctx context.Context,
	goTool GoTool,
	goRoot string,
	output string,
	target BuildTarget,
) (string, error) {
	if ctx == nil ||
		!filepath.IsAbs(goRoot) ||
		!filepath.IsAbs(output) ||
		!strings.HasPrefix(target.ImportPath, "./") {
		return "", errors.New("jobmgr build identity: invalid build request")
	}
	if err := requireRegular(
		filepath.Join(goRoot, "go.mod"),
		16*1024*1024,
	); err != nil {
		return "", errors.New(
			"jobmgr build identity: exported Go module is unavailable",
		)
	}
	if err := VerifyGoTool(ctx, goTool.Path, goTool.Version); err != nil {
		return "", err
	}
	if info, err := os.Stat(filepath.Dir(output)); err != nil ||
		!info.IsDir() {
		return "", errors.New(
			"jobmgr build identity: build output directory is unavailable",
		)
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		return "", errors.New(
			"jobmgr build identity: build output already exists",
		)
	}
	arguments := []string{"build", "-trimpath"}
	if target.Expectation.Tags != "" {
		arguments = append(
			arguments,
			"-tags="+target.Expectation.Tags,
		)
	}
	arguments = append(
		arguments,
		"-o",
		output,
		target.ImportPath,
	)
	command := exec.CommandContext(ctx, goTool.Path, arguments...)
	command.Dir = goRoot
	command.Env = GoEnvironment(map[string]string{
		"CGO_ENABLED": target.Expectation.CGO,
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf(
			"jobmgr build identity: build %s: %w: %s",
			target.ImportPath,
			err,
			strings.TrimSpace(stderr.String()),
		)
	}
	if stdout.Len() != 0 {
		return "", errors.New(
			"jobmgr build identity: Go build produced unexpected stdout",
		)
	}
	if err := VerifyExecutable(output, target.Expectation); err != nil {
		return "", err
	}
	return ArtifactSHA256(output)
}

func ArtifactSHA256(path string) (string, error) {
	return fileSHA256(path, maximumExecutableBytes)
}

func GoEnvironment(overrides map[string]string) []string {
	base := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + os.TempDir(),
		"LANG=C",
		"LC_ALL=C",
		"TZ=UTC",
		"GOMAXPROCS=4",
		"GOFLAGS=-mod=readonly",
		"GOTOOLCHAIN=local",
		"GOWORK=off",
	}
	environment := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if _, replace := overrides[name]; ok && replace {
			continue
		}
		environment = append(environment, entry)
	}
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		environment = append(
			environment,
			name+"="+overrides[name],
		)
	}
	return environment
}
