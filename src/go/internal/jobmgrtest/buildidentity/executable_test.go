package buildidentity

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"testing"
)

func TestCurrentGoToolMatchesRunningToolchain(t *testing.T) {
	tool, err := CurrentGoTool(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tool.Version != runtime.Version() {
		t.Fatalf(
			"Go tool version=%q, want %q",
			tool.Version,
			runtime.Version(),
		)
	}
	if err := VerifyGoTool(
		context.Background(),
		tool.Path,
		"go0.invalid",
	); err == nil {
		t.Fatal("mismatched Go tool version was accepted")
	}
}

func TestExecutableBuildIdentityRejectsWrongTargetSettings(t *testing.T) {
	expectation := ExecutableExpectation{
		Package: "example.test/phase",
		CGO:     "0",
		Tags:    "phase",
	}
	base := func() *debug.BuildInfo {
		return &debug.BuildInfo{
			GoVersion: runtime.Version(),
			Path:      expectation.Package,
			Settings: []debug.BuildSetting{
				{Key: "-buildmode", Value: "exe"},
				{Key: "-compiler", Value: "gc"},
				{Key: "-tags", Value: expectation.Tags},
				{Key: "-trimpath", Value: "true"},
				{Key: "CGO_ENABLED", Value: expectation.CGO},
				{Key: "GOARCH", Value: runtime.GOARCH},
				{Key: "GOOS", Value: runtime.GOOS},
			},
		}
	}
	tests := map[string]func(*debug.BuildInfo){
		"Go version": func(info *debug.BuildInfo) {
			info.GoVersion = "go0.invalid"
		},
		"package": func(info *debug.BuildInfo) {
			info.Path = "example.test/other"
		},
		"CGO": func(info *debug.BuildInfo) {
			info.Settings[4].Value = "1"
		},
		"tags": func(info *debug.BuildInfo) {
			info.Settings[2].Value = "other"
		},
		"trimpath": func(info *debug.BuildInfo) {
			info.Settings[3].Value = "false"
		},
	}
	if err := verifyExecutableBuildInfo(base(), expectation); err != nil {
		t.Fatalf("valid executable identity: %v", err)
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			info := base()
			mutate(info)
			if err := verifyExecutableBuildInfo(
				info,
				expectation,
			); err == nil {
				t.Fatal("wrong executable identity was accepted")
			}
		})
	}
}

func TestExecutableBuildIdentityAcceptsPinnedToolArtifact(t *testing.T) {
	tool, err := CurrentGoTool(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	module := t.TempDir()
	files := map[string]string{
		"go.mod":  "module example.test/identity\n\ngo 1.26\n",
		"main.go": "package main\nfunc main() {}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(
			filepath.Join(module, name),
			[]byte(content),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}
	executable := filepath.Join(t.TempDir(), "identity")
	command := exec.Command(
		tool.Path,
		"build",
		"-trimpath",
		"-tags=phase",
		"-o",
		executable,
		".",
	)
	command.Dir = module
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + os.TempDir(),
		"LANG=C",
		"LC_ALL=C",
		"CGO_ENABLED=0",
		"GOFLAGS=-mod=readonly",
		"GOTOOLCHAIN=local",
		"GOWORK=off",
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build pinned artifact: %v: %s", err, output)
	}
	if err := VerifyExecutable(
		executable,
		ExecutableExpectation{
			Package: "example.test/identity",
			CGO:     "0",
			Tags:    "phase",
		},
	); err != nil {
		t.Fatal(err)
	}
}
