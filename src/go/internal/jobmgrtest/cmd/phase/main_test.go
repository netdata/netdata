package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/buildidentity"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/contract"
)

func TestParseOptionsRequiresAbsolutePaths(t *testing.T) {
	required := map[string]string{
		"-go-root":            "/go",
		"-baseline-bundle":    "/baseline",
		"-evidence-directory": "/evidence",
		"-root-config-dir":    "/config",
	}
	base := make([]string, 0, len(required)*2)
	for name, value := range required {
		base = append(base, name, value)
	}
	tests := map[string]struct {
		arguments []string
		wantError string
	}{
		"complete": {
			arguments: base,
		},
		"relative": {
			arguments: append(
				append([]string(nil), base...),
				"-go-root",
				"relative",
			),
			wantError: "Go root must be absolute",
		},
		"positional": {
			arguments: append(
				append([]string(nil), base...),
				"extra",
			),
			wantError: "positional arguments are forbidden",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parseOptions(test.arguments)
			if test.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(
				err.Error(),
				test.wantError,
			) {
				t.Fatalf("error=%v, want containing %q", err, test.wantError)
			}
		})
	}
}

func TestRootConfigsRequireExplicitEmptyJobs(t *testing.T) {
	tests := map[string]struct {
		config    string
		wantError string
	}{
		"empty jobs": {
			config: "jobs: []\n",
		},
		"missing jobs": {
			config:    "{}\n",
			wantError: "must contain jobs: []",
		},
		"nonempty jobs": {
			config:    "jobs:\n  - name: unexpected\n",
			wantError: "must contain jobs: []",
		},
		"unknown field": {
			config:    "jobs: []\nunknown: true\n",
			wantError: "field unknown not found",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			for _, relative := range []string{
				"go.d/testrandom.conf",
				"ibm.d/websphere_mp.conf",
				"scripts.d/nagios.conf",
			} {
				path := filepath.Join(directory, relative)
				if err := os.MkdirAll(
					filepath.Dir(path),
					0o700,
				); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(
					path,
					[]byte(test.config),
					0o600,
				); err != nil {
					t.Fatal(err)
				}
			}
			err := validateRootConfigs(directory)
			if test.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(
				err.Error(),
				test.wantError,
			) {
				t.Fatalf("error=%v, want containing %q", err, test.wantError)
			}
		})
	}
}

func TestPackageGateManifestCoversApprovedPhase(t *testing.T) {
	cases, err := contract.BMM002Cases()
	if err != nil {
		t.Fatal(err)
	}
	gates, err := collectPackageGates(cases)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(contract.BMM003HotpathGates()); got != 35 {
		t.Fatalf("hotpath owners=%d, want 35", got)
	}
	if got := countBenchmarks(gates); got != 35 {
		t.Fatalf("hotpath benchmarks=%d, want 35", got)
	}
	jobmgr := gates["./plugin/agent/jobmgr"]
	if jobmgr == nil {
		t.Fatal("jobmgr package is absent")
	}
	for _, name := range []string{
		"TestActiveArchitecturePackages",
		"TestProductionConstructionChain",
	} {
		if _, ok := jobmgr.tests[name]; !ok {
			t.Fatalf("structural proof %s is absent", name)
		}
	}
}

func TestPackageGateNamesExist(t *testing.T) {
	cases, err := contract.BMM002Cases()
	if err != nil {
		t.Fatal(err)
	}
	gates, err := collectPackageGates(cases)
	if err != nil {
		t.Fatal(err)
	}
	goRoot, err := filepath.Abs(filepath.Join(
		"..",
		"..",
		"..",
		"..",
	))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(
		context.Background(),
		2*time.Minute,
	)
	defer cancel()
	goTool, err := buildidentity.CurrentGoTool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyNamedGates(
		ctx,
		goTool.Path,
		goRoot,
		gates,
	); err != nil {
		t.Fatal(err)
	}
}

func TestExactNamePatternAndListParser(t *testing.T) {
	tests := map[string]struct {
		names  []string
		output string
		want   map[string]struct{}
	}{
		"metacharacters": {
			names: []string{"TestA/x", "BenchmarkB+"},
			want: map[string]struct{}{
				"TestA/x":     {},
				"BenchmarkB+": {},
			},
		},
		"go list output": {
			output: "TestA\nBenchmarkB\nExampleC\nok  pkg  0.1s\n",
			want: map[string]struct{}{
				"TestA":      {},
				"BenchmarkB": {},
				"ExampleC":   {},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.output != "" {
				if got := parseGoTestList(test.output); !reflect.DeepEqual(
					got,
					test.want,
				) {
					t.Fatalf("list=%v, want %v", got, test.want)
				}
				return
			}
			pattern := exactNamePattern(test.names)
			for candidate := range test.want {
				if matched, err := regexpMatch(
					pattern,
					candidate,
				); err != nil || !matched {
					t.Fatalf(
						"pattern=%q candidate=%q matched=%v err=%v",
						pattern,
						candidate,
						matched,
						err,
					)
				}
			}
		})
	}
}

func TestPhaseGoEnvironmentIsFixedAndOverrideable(t *testing.T) {
	got := phaseGoEnvironment(
		map[string]string{
			"CGO_ENABLED": "0",
			"GOMAXPROCS":  "99",
		},
	)
	joined := strings.Join(got, "\n")
	for _, required := range []string{
		"LANG=C",
		"LC_ALL=C",
		"TZ=UTC",
		"GOFLAGS=-mod=readonly",
		"GOTOOLCHAIN=local",
		"GOWORK=off",
		"CGO_ENABLED=0",
		"GOMAXPROCS=99",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("environment lacks %q: %v", required, got)
		}
	}
	if strings.Count(joined, "GOMAXPROCS=") != 1 {
		t.Fatalf("GOMAXPROCS override is duplicated: %v", got)
	}
}

func TestPhaseBuildTargetsOwnEverySupportedRoot(t *testing.T) {
	artifacts := phaseArtifacts{
		production:     "/build/agent",
		godplugin:      "/build/go.d.plugin",
		ibmdplugin:     "/build/ibm.d.plugin",
		scriptsdplugin: "/build/scripts.d.plugin",
	}
	targets := phaseBuildTargets(artifacts)
	tests := map[string]phaseBuildTarget{
		"production Agent driver": {
			path: "/build/agent", importPath: "./internal/jobmgrtest/cmd/agent",
			cgo: "0",
		},
		"go.d.plugin": {
			path: "/build/go.d.plugin", importPath: "./cmd/godplugin",
			cgo: "0",
		},
		"ibm.d.plugin": {
			path: "/build/ibm.d.plugin", importPath: "./cmd/ibmdplugin",
			tags: "ibm_mq", cgo: "1",
		},
		"scripts.d.plugin": {
			path:       "/build/scripts.d.plugin",
			importPath: "./cmd/scriptsdplugin",
			cgo:        "0",
		},
	}
	if !reflect.DeepEqual(targets, tests) {
		t.Fatalf("phase build targets=%v, want %v", targets, tests)
	}
}

func TestExecutableIdentityRejectsEqualContent(t *testing.T) {
	directory := t.TempDir()
	paths := map[string]string{
		"left":  filepath.Join(directory, "left"),
		"right": filepath.Join(directory, "right"),
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("same"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	same, err := sameExecutableContent(paths["left"], paths["right"])
	if err != nil {
		t.Fatal(err)
	}
	if !same {
		t.Fatal("equal executable content was not detected")
	}
	if err := os.WriteFile(
		paths["right"],
		[]byte("different"),
		0o700,
	); err != nil {
		t.Fatal(err)
	}
	same, err = sameExecutableContent(paths["left"], paths["right"])
	if err != nil {
		t.Fatal(err)
	}
	if same {
		t.Fatal("different executable content was reported equal")
	}
}

func regexpMatch(pattern, candidate string) (bool, error) {
	return regexp.MatchString(pattern, candidate)
}
