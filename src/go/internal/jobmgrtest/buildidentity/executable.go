package buildidentity

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
)

type GoTool struct {
	Path    string
	Version string
}

type ExecutableExpectation struct {
	Package string
	CGO     string
	Tags    string
}

func CurrentGoTool(ctx context.Context) (GoTool, error) {
	if ctx == nil {
		return GoTool{}, errors.New("jobmgr build identity: nil Go tool context")
	}
	probe := ""
	if root := runtime.GOROOT(); root != "" {
		candidate := filepath.Join(root, "bin", "go")
		if validateToolExecutable(candidate) == nil {
			probe = candidate
		}
	}
	if probe == "" {
		var err error
		probe, err = exec.LookPath("go")
		if err != nil {
			return GoTool{}, errors.New(
				"jobmgr build identity: Go tool is unavailable",
			)
		}
	}
	if err := validateToolExecutable(probe); err != nil {
		return GoTool{}, err
	}
	if err := VerifyGoTool(ctx, probe, runtime.Version()); err != nil {
		return GoTool{}, err
	}
	root, err := goToolRoot(ctx, probe)
	if err != nil {
		return GoTool{}, err
	}
	path := filepath.Join(root, "bin", "go")
	if err := validateToolExecutable(path); err != nil {
		return GoTool{}, err
	}
	if err := VerifyGoTool(ctx, path, runtime.Version()); err != nil {
		return GoTool{}, err
	}
	return GoTool{Path: path, Version: runtime.Version()}, nil
}

func VerifyGoTool(
	ctx context.Context,
	executable string,
	version string,
) error {
	if ctx == nil || executable == "" || version == "" {
		return errors.New("jobmgr build identity: invalid Go tool identity")
	}
	command := exec.CommandContext(ctx, executable, "version")
	command.Env = []string{"LANG=C", "LC_ALL=C"}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf(
			"jobmgr build identity: Go tool version: %w: %s",
			err,
			strings.TrimSpace(stderr.String()),
		)
	}
	fields := strings.Fields(stdout.String())
	if len(fields) < 3 || fields[0] != "go" ||
		fields[1] != "version" || fields[2] != version {
		return fmt.Errorf(
			"jobmgr build identity: Go tool version %q differs from %q",
			strings.TrimSpace(stdout.String()),
			version,
		)
	}
	return nil
}

func goToolRoot(ctx context.Context, executable string) (string, error) {
	command := exec.CommandContext(ctx, executable, "env", "GOROOT")
	command.Env = []string{"LANG=C", "LC_ALL=C"}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf(
			"jobmgr build identity: Go tool root: %w: %s",
			err,
			strings.TrimSpace(stderr.String()),
		)
	}
	root := strings.TrimSpace(stdout.String())
	if !filepath.IsAbs(root) {
		return "", errors.New(
			"jobmgr build identity: Go tool root is not absolute",
		)
	}
	return root, nil
}

func VerifyExecutable(
	executable string,
	expectation ExecutableExpectation,
) error {
	if err := validateToolExecutable(executable); err != nil {
		return err
	}
	info, err := buildinfo.ReadFile(executable)
	if err != nil {
		return fmt.Errorf(
			"jobmgr build identity: read executable build info: %w",
			err,
		)
	}
	return verifyExecutableBuildInfo(info, expectation)
}

func verifyExecutableBuildInfo(
	info *debug.BuildInfo,
	expectation ExecutableExpectation,
) error {
	if info == nil || expectation.Package == "" ||
		(expectation.CGO != "0" && expectation.CGO != "1") {
		return errors.New(
			"jobmgr build identity: invalid executable expectation",
		)
	}
	if info.GoVersion != runtime.Version() {
		return fmt.Errorf(
			"jobmgr build identity: executable Go version %q differs from %q",
			info.GoVersion,
			runtime.Version(),
		)
	}
	if info.Path != expectation.Package {
		return fmt.Errorf(
			"jobmgr build identity: executable package %q differs from %q",
			info.Path,
			expectation.Package,
		)
	}
	settings := make(map[string]string, len(info.Settings))
	for _, setting := range info.Settings {
		if _, ok := settings[setting.Key]; ok {
			return fmt.Errorf(
				"jobmgr build identity: duplicate build setting %q",
				setting.Key,
			)
		}
		settings[setting.Key] = setting.Value
	}
	required := map[string]string{
		"-buildmode":  "exe",
		"-compiler":   "gc",
		"-trimpath":   "true",
		"CGO_ENABLED": expectation.CGO,
		"GOARCH":      runtime.GOARCH,
		"GOOS":        runtime.GOOS,
	}
	for name, want := range required {
		if got := settings[name]; got != want {
			return fmt.Errorf(
				"jobmgr build identity: executable setting %s=%q, want %q",
				name,
				got,
				want,
			)
		}
	}
	if got := settings["-tags"]; got != expectation.Tags {
		return fmt.Errorf(
			"jobmgr build identity: executable tags %q, want %q",
			got,
			expectation.Tags,
		)
	}
	return nil
}

func validateToolExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() ||
		info.Mode()&0o111 == 0 {
		return errors.New(
			"jobmgr build identity: executable is unavailable",
		)
	}
	return nil
}
