package main

import (
	"strings"
	"testing"
)

func TestParseOptionsRequiresCommittedGoRootAndAbsoluteEvidencePaths(
	t *testing.T,
) {
	base := []string{
		"-go-root", "/go",
		"-baseline-bundle", "/baseline",
		"-evidence-directory", "/evidence",
	}
	tests := map[string]struct {
		arguments []string
		wantError string
	}{
		"complete": {
			arguments: base,
		},
		"relative Go root": {
			arguments: append(
				append([]string(nil), base...),
				"-go-root",
				"relative",
			),
			wantError: "Go root must be absolute",
		},
		"caller supplied executable": {
			arguments: append(
				append([]string(nil), base...),
				"-production-executable",
				"/candidate",
			),
			wantError: "flag provided but not defined",
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
				t.Fatalf(
					"error=%v, want containing %q",
					err,
					test.wantError,
				)
			}
		})
	}
}
