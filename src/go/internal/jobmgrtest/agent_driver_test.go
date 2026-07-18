package jobmgrtest

import (
	"context"
	"testing"
	"time"
)

func TestAgentDriverUsesPublicLifecycle(t *testing.T) {
	tests := map[string]struct {
		scenario string
	}{
		"collector generation": {
			scenario: "collector-generation",
		},
		"collector acquired abort": {
			scenario: "collector-acquired-abort",
		},
		"Function publication abort": {
			scenario: "function-publication-abort",
		},
		"bounded shutdown": {
			scenario: "bounded-shutdown",
		},
		"atomic output": {
			scenario: "atomic-output",
		},
		"Function result smoke": {
			scenario: "function-result-smoke",
		},
		"Function header boundaries": {
			scenario: "function-header-boundaries",
		},
		"Function body boundaries": {
			scenario: "function-body-boundaries",
		},
		"Function raw payload": {
			scenario: "function-raw-payload",
		},
		"Function timeout boundaries": {
			scenario: "function-timeout-boundaries",
		},
		"Function invalid JSON": {
			scenario: "function-invalid-json",
		},
		"Function result boundaries": {
			scenario: "function-result-boundaries",
		},
		"bounded Function flow": {
			scenario: "bounded-function-flow",
		},
	}
	driver := &AgentDriver{}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				60*time.Second,
			)
			defer cancel()
			if err := driver.Run(ctx, test.scenario); err != nil {
				t.Fatal(err)
			}
		})
	}
}
